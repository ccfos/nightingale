package evallog

import (
	"fmt"
	"sync"
	"time"
)

// Config evallog 配置，对应 [Alert.EvalLog] 配置段。
// 默认启用，Disable=true 关闭（对齐 Alert.Disable 的语义，配置段缺省即启用）。
type Config struct {
	Disable            bool
	Dir                string // 记录目录，缺省时由 aconf.Alert.PreCheck 填成 <[Log] Dir>/evallog
	RetentionHours     int    // 保留时长，默认 192（8 天）
	MaxSeriesPerQuery  int    // 闸1a：单查询保留的原始曲线条数，默认 100
	MaxPointsPerSeries int    // 闸1b：单曲线保留的点数，默认 60
	MaxRecordBytes     int    // 闸1c：单条记录序列化后硬上限，默认 256KB
	PerRuleDailyMB     int    // 闸2：单规则单日写入预算（未压缩，跨该规则命中的全部数据源合计），超出后当日降级为摘要，默认 1024
	QueueSize          int    // 闸3：异步写入队列长度，默认 512（同时是磁盘卡顿时的最坏内存驻留系数，见 defQueueSize）
	MaxDiskGB          int    // 兜底：目录总量上限，超出从最旧小时删起，默认 20

	// 以下两个是**读侧**的闸，防的是「查询把告警引擎拖垮」。
	// 写侧的闸只保证单条记录不失控，挡不住一次查询把几百 MB 记录同时拉进堆里。
	MaxQueryBytes        int // 闸4：单次查询返回记录的序列化字节上限，默认 32MB
	MaxConcurrentQueries int // 闸5：本进程同时执行的查询数上限，默认 2
}

const (
	// defDir 只在绕过配置层（直接构造 Config）且 [Log] Dir 也为空时兜底，
	// 正常路径的默认值由 aconf.Alert.PreCheck 按 [Log] Dir 拼出来。
	defDir                = "./evallog"
	defRetentionHours     = 192
	defMaxSeriesPerQuery  = 100
	defMaxPointsPerSeries = 60
	defMaxRecordBytes     = 256 * 1024
	defPerRuleDailyMB     = 1024
	// defQueueSize 写入队列长度。
	//
	// 队列里是**序列化前**的 *EvalRecord，满载一条（100 曲线 × 60 点 + labels）在内存中约
	// 200KB+：磁盘 hang/NFS 卡顿把消费 goroutine 堵住时，队列有多深、堆就要陪多深——
	// 4096 意味着最坏 GB 级驻留，小内存 edge 节点会被 OOM 反杀，旁路功能拖垮主流程。
	// 512 给消费端约一秒的卡顿余量就够了（正常水位近零：入队每秒几百条 vs 消费每秒数千条，
	// 1 万 rule-ds × 15s 间隔也只有 667 条/秒），最坏驻留收敛到 ~100MB。
	// 被丢弃的记录有 dropHook 计数 + info 摘要回退兜底，不会无声消失；超大部署可自行调高。
	defQueueSize = 512
	defMaxDiskGB = 20

	// defMaxQueryBytes 单次查询字节预算。
	//
	// 32MB 是按实测记录大小定的，不是拍脑袋的整数：默认闸下一条"满载"记录（100 曲线 ×
	// 60 点）落盘约 145KB，常见规则（5 曲线）约 8KB、中等规则（20 曲线）约 33KB。
	// 前端一页取 1000 条，于是
	//   - 常见规则整页约 8MB、中等规则整页约 33MB：都在预算内或刚好压线，日常翻页不受影响；
	//   - 满载规则整页则是 145MB，解码后还要再翻几倍——这正是要挡住的那一类。预算生效时
	//     返回最新的约 220 条并带上截断说明，而不是默默少给几百条。
	// 抬高它之前先算一遍 MaxConcurrentQueries × 本值 × 3.7：实测解码后的 EvalRecord 约为
	// 落盘字节的 2.7 倍，handler 再 json.Marshal 一份，这才是告警引擎要额外承受的堆
	//（实测满载场景：预算 32MB 时单次查询分配约 116MB，关掉预算则是 160MB 且随数据量无上限）。
	// 另外它不能超过 center 侧的 evalRecordsMaxRespBytes。
	defMaxQueryBytes = 32 * 1024 * 1024
	// defMaxConcurrentQueries 并发查询上限。
	//
	// 这个进程的主业是评估告警规则，查询是排障时的旁路操作，2 路并发已经够用（单次查询
	// 通常几十毫秒）。真正的作用是给内存和 CPU 封顶：没有它，N 个用户同时刷新就是 N 倍的
	// MaxQueryBytes + N 倍的 gzip 解压，评估协程会被 GC 和 CPU 一起挤住。
	defMaxConcurrentQueries = 2
)

func (c *Config) normalize() {
	if c.Dir == "" {
		c.Dir = defDir
	}
	if c.RetentionHours <= 0 {
		c.RetentionHours = defRetentionHours
	}
	if c.MaxSeriesPerQuery <= 0 {
		c.MaxSeriesPerQuery = defMaxSeriesPerQuery
	}
	if c.MaxPointsPerSeries <= 0 {
		c.MaxPointsPerSeries = defMaxPointsPerSeries
	}
	if c.MaxRecordBytes <= 0 {
		c.MaxRecordBytes = defMaxRecordBytes
	}
	if c.PerRuleDailyMB <= 0 {
		c.PerRuleDailyMB = defPerRuleDailyMB
	}
	if c.QueueSize <= 0 {
		c.QueueSize = defQueueSize
	}
	if c.MaxDiskGB <= 0 {
		c.MaxDiskGB = defMaxDiskGB
	}
	if c.MaxQueryBytes <= 0 {
		c.MaxQueryBytes = defMaxQueryBytes
	}
	if c.MaxConcurrentQueries <= 0 {
		c.MaxConcurrentQueries = defMaxConcurrentQueries
	}
}

var (
	mu            sync.RWMutex
	defaultWriter *Writer
	stopCleaner   chan struct{}
	querySem      chan struct{}
	hooks         Hooks
)

// Hooks 供调用方上报指标的回调，各字段可为 nil。
type Hooks struct {
	OnDrop        func() // 记录因队列满/降级后仍超限被丢弃
	OnQueryReject func() // 查询因并发闸被拒
}

// ErrNotEnabled evallog 未启用（配置关闭或该进程未初始化告警引擎）。
var ErrNotEnabled = fmt.Errorf("evallog not enabled")

// ErrBusy 查询被并发闸拒绝。
//
// 必须与"没有记录"严格区分：查询接口的语义是回答"当时到底发生了什么"，返回空列表会被
// 直接读成"当时确实没评估"。所以这里宁可给一个明确的、可重试的错误，也不返回空结果。
var ErrBusy = fmt.Errorf("evallog is busy: too many concurrent eval-record queries on this engine instance, please retry")

// queryAcquireTimeout 并发闸的排队等待上限。
//
// 不做「拿不到就立刻拒」：单次查询通常几十毫秒，几个用户同时点开抽屉属于常态，排一下队
// 就过去了。也不能等太久——center 侧转发的超时是 5s，等超过它只会让调用方拿到一个含糊的
// 客户端超时，而不是这里明确的 ErrBusy。
// 变量而非常量：测试需要把它调小，避免用例空等 2 秒。
var queryAcquireTimeout = 2 * time.Second

// Init 初始化包级单例：启动异步写入与清理协程。
// 重复调用会先关闭旧实例（主要服务于测试）。
func Init(cfg Config, h Hooks) error {
	mu.Lock()
	defer mu.Unlock()

	if defaultWriter != nil {
		defaultWriter.Close()
		defaultWriter = nil
	}
	if stopCleaner != nil {
		close(stopCleaner)
		stopCleaner = nil
	}
	querySem = nil
	hooks = h

	if cfg.Disable {
		return nil
	}

	cfg.normalize()
	w, err := NewWriter(cfg, h.OnDrop)
	if err != nil {
		return err
	}
	defaultWriter = w
	querySem = make(chan struct{}, cfg.MaxConcurrentQueries)
	stopCleaner = make(chan struct{})
	go cleanLoop(cfg, stopCleaner)
	return nil
}

// Enabled 是否已启用（Init 成功且未关闭）。
func Enabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return defaultWriter != nil
}

func currentConfig() *Config {
	mu.RLock()
	defer mu.RUnlock()
	if defaultWriter == nil {
		return nil
	}
	return &defaultWriter.cfg
}

// SeriesCap 单查询原始曲线保留上限，供采集侧提前截断避免无谓分配；未启用时返回 0。
func SeriesCap() int {
	cfg := currentConfig()
	if cfg == nil {
		return 0
	}
	return cfg.MaxSeriesPerQuery
}

// PointsCap 单曲线保留的点数上限，供采集侧提前截断避免无谓分配；未启用时返回 0。
//
// 与 SeriesCap 成对：采集侧若先把整条曲线的点全复制一份、再交给 AddQuery 从尾部重新切片，
// Go 会因为 slice 仍指向那块分配而保留**整份**原始点数组——闸1b 就只限住了序列化长度，
// 限不住堆。非 prom 数据源的 range 查询单曲线点数可达千级，100 条曲线就是十几 MB，
// 且记录在写入队列里排队期间一直不释放。
func PointsCap() int {
	cfg := currentConfig()
	if cfg == nil {
		return 0
	}
	return cfg.MaxPointsPerSeries
}

// QueryConcurrency 本进程允许的并发查询数（闸5）；未启用时返回 0。
//
// 给同进程的调用方（center 与 alert 合并部署时，center 的扇出会直接打到本地）用：
// 扇出并发若大于这个数，多出来的那些查询只会排队到超时、拿一个 ErrBusy，白白把一次
// 正常的页面请求变成半屏"引擎忙"。按它对齐本地扇出即可完全避免这种自伤。
func QueryConcurrency() int {
	cfg := currentConfig()
	if cfg == nil {
		return 0
	}
	return cfg.MaxConcurrentQueries
}

// Push 异步提交一条记录，nil 记录或未启用时 no-op。
// 返回是否成功入队：队列满被丢弃时为 false，调用方可据此回退到日志输出完整现场，
// 避免「现场日志已降级为 debug + 记录又没落盘」两头落空。
func Push(r *EvalRecord) bool {
	if r == nil {
		return false
	}
	mu.RLock()
	w := defaultWriter
	mu.RUnlock()
	if w == nil {
		return false
	}
	return w.Push(r)
}

// QueryRecords 查询本节点某规则在 [fromMs, toMs] 内的评估记录，按 ts 倒序。
// beforeMs > 0 时仅返回 ts < beforeMs 的记录（翻页游标）。
// 受并发闸约束，排队超时返回 ErrBusy；结果受字节预算约束，见 QueryResult.Truncated。
func QueryRecords(ruleId, datasourceId int64, fromMs, toMs, beforeMs int64, limit int) (QueryResult, error) {
	mu.RLock()
	w := defaultWriter
	sem := querySem
	onReject := hooks.OnQueryReject
	mu.RUnlock()
	if w == nil || sem == nil {
		return QueryResult{}, ErrNotEnabled
	}

	// 释放用的是这里捕获的 sem：期间若发生重新 Init，包级 querySem 已换成新的，
	// 往新信号量里还令牌会把它的容量凭空撑大
	if !acquireQuerySlot(sem) {
		if onReject != nil {
			onReject()
		}
		return QueryResult{}, ErrBusy
	}
	defer func() { <-sem }()

	return queryRecords(w.cfg, ruleId, datasourceId, fromMs, toMs, beforeMs, limit)
}

func acquireQuerySlot(sem chan struct{}) bool {
	select {
	case sem <- struct{}{}:
		return true
	default:
	}
	timer := time.NewTimer(queryAcquireTimeout)
	defer timer.Stop()
	select {
	case sem <- struct{}{}:
		return true
	case <-timer.C:
		return false
	}
}

// Shutdown 关闭单例，刷盘退出（测试与进程退出用）。
func Shutdown() {
	mu.Lock()
	defer mu.Unlock()
	if defaultWriter != nil {
		defaultWriter.Close()
		defaultWriter = nil
	}
	if stopCleaner != nil {
		close(stopCleaner)
		stopCleaner = nil
	}
	querySem = nil
}
