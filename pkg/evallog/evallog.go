package evallog

import (
	"fmt"
	"sync"
)

// Config evallog 配置，对应 [Alert.EvalLog] 配置段。
// 默认启用，Disable=true 关闭（对齐 Alert.Disable 的语义，配置段缺省即启用）。
type Config struct {
	Disable            bool
	Dir                string // 记录目录，默认 ./evallog
	RetentionHours     int    // 保留时长，默认 192（8 天）
	MaxSeriesPerQuery  int    // 闸1a：单查询保留的原始曲线条数，默认 100
	MaxPointsPerSeries int    // 闸1b：单曲线保留的点数，默认 60
	MaxRecordBytes     int    // 闸1c：单条记录序列化后硬上限，默认 256KB
	PerRuleDailyMB     int    // 闸2：单规则单日写入预算（未压缩），超出后当日降级为摘要，默认 1024
	QueueSize          int    // 闸3：异步写入队列长度，默认 4096
	MaxDiskGB          int    // 兜底：目录总量上限，超出从最旧小时删起，默认 20
}

const (
	defDir                = "./evallog"
	defRetentionHours     = 192
	defMaxSeriesPerQuery  = 100
	defMaxPointsPerSeries = 60
	defMaxRecordBytes     = 256 * 1024
	defPerRuleDailyMB     = 1024
	defQueueSize          = 4096
	defMaxDiskGB          = 20
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
}

var (
	mu            sync.RWMutex
	defaultWriter *Writer
	stopCleaner   chan struct{}
)

// ErrNotEnabled evallog 未启用（配置关闭或该进程未初始化告警引擎）。
var ErrNotEnabled = fmt.Errorf("evallog not enabled")

// Init 初始化包级单例：启动异步写入与清理协程。
// dropHook 在记录因队列满被丢弃时调用（用于指标上报），可为 nil。
// 重复调用会先关闭旧实例（主要服务于测试）。
func Init(cfg Config, dropHook func()) error {
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

	if cfg.Disable {
		return nil
	}

	cfg.normalize()
	w, err := NewWriter(cfg, dropHook)
	if err != nil {
		return err
	}
	defaultWriter = w
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

// Push 异步提交一条记录，nil 记录或未启用时 no-op。
func Push(r *EvalRecord) {
	if r == nil {
		return
	}
	mu.RLock()
	w := defaultWriter
	mu.RUnlock()
	if w == nil {
		return
	}
	w.Push(r)
}

// QueryRecords 查询本节点某规则在 [fromMs, toMs] 内的评估记录，按 ts 倒序。
// beforeMs > 0 时仅返回 ts < beforeMs 的记录（翻页游标）。
func QueryRecords(ruleId, datasourceId int64, fromMs, toMs, beforeMs int64, limit int) ([]EvalRecord, error) {
	mu.RLock()
	w := defaultWriter
	mu.RUnlock()
	if w == nil {
		return nil, ErrNotEnabled
	}
	return queryRecords(w.cfg, ruleId, datasourceId, fromMs, toMs, beforeMs, limit)
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
}
