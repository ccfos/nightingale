package evallog

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/toolkits/pkg/logger"
)

const (
	dateLayout = "2006-01-02"
	hourLayout = "15"

	// 同时保持打开的追加文件句柄上限，超出后关闭最久未写的
	maxOpenAppenders = 512
	// 空闲超过该时长的句柄在周期检查时关闭（规则已停止或迁移）
	appenderIdleTimeout = 5 * time.Minute
	// 周期性刷盘/滚动检查间隔
	flushInterval = time.Second
	// 闸2 的当日预算计数在无写入多久后回收；必须大于一天，否则跨日前就被清掉
	budgetIdleTimeout = 25 * time.Hour
)

// writeFailCooldown 写失败熔断（见 markDegraded）的时长。
// 变量而非常量：测试需要把它调小，避免用例空等 30 秒。
var writeFailCooldown = 30 * time.Second

// Writer 单 goroutine 消费队列、按 {rule_ds}/{date}/{hour}.jsonl 追加写入，
// 整点滚动后异步 gzip。所有 appender / budgets 状态仅在消费 goroutine 内访问，无锁。
type Writer struct {
	cfg      Config
	ch       chan *EvalRecord
	done     chan struct{} // 消费 goroutine 退出信号
	closing  sync.Once
	dropHook func()

	appenders map[string]*appender   // key: {rule_id}_{ds_id}
	budgets   map[string]*ruleBudget // key: {rule_id}（跨该规则的全部数据源共用）；生命周期独立于 appender

	// gzMu 同时保护 gzClosed、gzInflight 与「向 gzCh 投递」：投递方除消费 goroutine 外
	// 还有启动时的 sweepLeftovers，若不互斥，Close 关闭 gzCh 后的残余投递会 panic
	//（向已关闭 channel 发送），gzWg.Add 与 gzWg.Wait 并发也会 panic。
	gzMu     sync.Mutex
	gzClosed bool
	gzCh     chan string
	gzWg     sync.WaitGroup
	// gzInflight 记录已排队/正在压缩的路径 → 完成信号，供 getAppender 在重开同名文件前
	// 等待，避免压缩任务的 os.Remove 删掉刚被重新打开的 inode（详见 waitGzDone）。
	gzInflight map[string]chan struct{}

	// 写失败熔断（见 markDegraded）。degradedUntilNs 由消费 goroutine 写、Push 在
	// eval goroutine 读，两个字段都必须原子访问。
	degradedUntilNs int64 // 熔断截止（UnixNano），0 表示未熔断
	degradedDrops   int64 // 熔断以来丢弃的记录数，恢复/续期日志用
}

type appender struct {
	ruleDs    string
	hourPath  string // 当前打开文件的完整路径
	hourKey   string // "2006-01-02/15"
	f         *os.File
	bw        *bufio.Writer
	lastWrite time.Time
}

// ruleBudget 闸2：单规则单日写入预算计数。
//
// 刻意不挂在 appender 上：appender 会因空闲 5 分钟（appenderIdleTimeout）或句柄数超过
// maxOpenAppenders 被关闭并从 map 删除，计数跟着一起没了。评估间隔大于 5 分钟的规则
// 每两次评估之间必然被关一次，预算永远累加不到门限，PerRuleDailyMB 等于完全失效。
//
// 也刻意**只按 rule_id 计**，而不是跟着 appender 用 {rule_id}_{ds_id}：一条按 cate 匹配的
// 规则可以命中几十个数据源，按组合键计等于把预算放大了同样的倍数（30 个数据源就是
// 30 × PerRuleDailyMB），闸2 先于 MaxDiskGB 失效。而 MaxDiskGB 的兜底是**从最旧小时全局
// 删起**，一条胖规则会把其他规则的历史记录一起挤掉，「保留 RetentionHours」的承诺就没了。
type ruleBudget struct {
	date    string // "2006-01-02"
	bytes   int64
	lastUse time.Time
}

func NewWriter(cfg Config, dropHook func()) (*Writer, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("evallog dir is blank")
	}
	if err := os.MkdirAll(cfg.Dir, 0755); err != nil {
		return nil, fmt.Errorf("evallog mkdir %s: %v", cfg.Dir, err)
	}

	w := &Writer{
		cfg:        cfg,
		ch:         make(chan *EvalRecord, cfg.QueueSize),
		done:       make(chan struct{}),
		dropHook:   dropHook,
		appenders:  make(map[string]*appender),
		budgets:    make(map[string]*ruleBudget),
		gzCh:       make(chan string, 1024),
		gzInflight: make(map[string]chan struct{}),
	}

	w.gzWg.Add(1)
	go w.gzLoop()
	go w.loop()
	go w.sweepLeftovers()
	return w, nil
}

// Push 非阻塞入队，队列满或写路径处于熔断期则丢弃并回调 dropHook。返回是否入队成功。
func (w *Writer) Push(r *EvalRecord) (queued bool) {
	// Close 与并发 Push 竞争时向已关闭 channel 发送会 panic，吞掉即可：
	// 关闭发生在进程退出/重新 Init，丢这一条记录无影响
	defer func() {
		if recover() != nil {
			queued = false
		}
	}()
	// 写失败熔断期内直接按「未入队」处理：调用方会走与队列满相同的 info 摘要回退，
	// 磁盘故障期间现场不至于两头落空；丢弃同样计入指标。
	if w.degraded(time.Now()) {
		atomic.AddInt64(&w.degradedDrops, 1)
		if w.dropHook != nil {
			w.dropHook()
		}
		return false
	}
	select {
	case w.ch <- r:
		return true
	default:
		if w.dropHook != nil {
			w.dropHook()
		}
		return false
	}
}

// Close 停止接收、清空队列、刷盘并等待压缩任务完成。
func (w *Writer) Close() {
	w.closing.Do(func() {
		close(w.ch)
		<-w.done

		// 置位 gzClosed 与 close(gzCh) 必须在同一把锁里，且 Wait 放在解锁之后：
		// 这样 enqueueGz 要么在置位前完成 Add+投递，要么看到 gzClosed 直接返回，
		// 不会出现「向已关闭 channel 发送」或「Add 与 Wait 并发」。
		w.gzMu.Lock()
		w.gzClosed = true
		close(w.gzCh)
		w.gzMu.Unlock()

		w.gzWg.Wait()
	})
}

// gzStopped 供 sweepLeftovers 提前中止：Writer 已关闭后再遍历目录纯属浪费。
func (w *Writer) gzStopped() bool {
	w.gzMu.Lock()
	defer w.gzMu.Unlock()
	return w.gzClosed
}

func (w *Writer) loop() {
	defer close(w.done)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case r, ok := <-w.ch:
			if !ok {
				// 排空后收尾：只刷盘关句柄，当前小时文件不压缩（重启后继续追加）
				w.closeAll(false)
				return
			}
			w.write(r)
		case <-ticker.C:
			w.housekeep()
		}
	}
}

func (w *Writer) write(r *EvalRecord) {
	now := time.Now()
	// 熔断期内直接丢弃并计数：不 marshal、不碰磁盘、不逐条打日志。
	// Push 侧的闸只拦新记录，队列里可能还留着熔断生效前入队的存量，这里兜住它们。
	if w.degraded(now) {
		w.dropDegraded()
		return
	}

	key := fmt.Sprintf("%d_%d", r.RuleId, r.DatasourceId)

	line, ok := w.marshalWithinLimit(r)
	if !ok {
		return
	}

	a := w.getAppender(key, r)
	if a == nil {
		// getAppender 已打过带路径与原因的日志，这里负责计数并进入熔断
		w.dropDegraded()
		w.markDegraded(now, "open hour file failed")
		return
	}

	// 闸2：单规则单日预算（跨该规则的全部数据源合计），超出后降级为摘要模式
	b := w.budgetFor(strconv.FormatInt(r.RuleId, 10), msTime(r.Ts).Format(dateLayout), now)
	limit := int64(w.cfg.PerRuleDailyMB) * 1024 * 1024
	if b.bytes+int64(len(line)) > limit && (len(r.Queries) > 0 || len(r.Events) > 0) {
		// 超预算后降级为摘要模式：同样先丢曲线，事件轨迹保留骨架
		r.stripSeries()
		r.stripEventDetails()
		var err error
		if line, err = json.Marshal(r); err != nil {
			w.dropRecord(r, "marshal after budget degrade", err)
			return
		}
	}

	if _, err := a.bw.Write(append(line, '\n')); err != nil {
		// bufio 的 error 会粘滞在句柄上，继续复用只会让后续写全部失败，关掉并进入熔断
		w.finalize(a, false)
		w.dropDegraded()
		w.markDegraded(now, err.Error())
		return
	}
	// 预算在写成功后才累加：写失败/熔断丢弃的记录不该虚占当日额度
	b.bytes += int64(len(line))
	a.lastWrite = now

	// 熔断到期后的首次成功写入：恢复并收口统计
	if atomic.LoadInt64(&w.degradedUntilNs) != 0 {
		logger.Infof("evallog write path recovered, %d records were dropped during degradation",
			atomic.LoadInt64(&w.degradedDrops))
		atomic.StoreInt64(&w.degradedUntilNs, 0)
		atomic.StoreInt64(&w.degradedDrops, 0)
	}
}

// degraded 写路径是否处于熔断期。
func (w *Writer) degraded(now time.Time) bool {
	return now.UnixNano() < atomic.LoadInt64(&w.degradedUntilNs)
}

// dropDegraded 计一条熔断相关丢弃（指标 + 恢复日志的统计），刻意不打日志。
func (w *Writer) dropDegraded() {
	atomic.AddInt64(&w.degradedDrops, 1)
	if w.dropHook != nil {
		w.dropHook()
	}
}

// markDegraded 进入/续期写失败熔断。
//
// 磁盘满/hang 几乎总是盘级故障而非单文件故障，所以按 Writer 全局熔断，不按 appender 区分。
// 没有它，磁盘满之后 bufio 的粘滞 error 会让**每条**记录都产出一条 Warning——日志和
// evallog 常在同一块盘上，等于在磁盘最紧张的时刻再压上一份写放大；getAppender 失败路径
// 还会每条记录白做一次 MkdirAll+OpenFile。熔断期内 Push 直接拒绝（调用方回退 info 摘要，
// 现场不丢）、write 入口直接丢弃计数，每个窗口只有进入/续期时这一条日志。
// 到期后由下一条记录探测：写成功即恢复，再失败则续期。
func (w *Writer) markDegraded(now time.Time, reason string) {
	atomic.StoreInt64(&w.degradedUntilNs, now.Add(writeFailCooldown).UnixNano())
	logger.Warningf("evallog write path degraded (%s): dropping eval records for %s, %d dropped since degradation began; check disk space/health of %s",
		reason, writeFailCooldown, atomic.LoadInt64(&w.degradedDrops), w.cfg.Dir)
}

// marshalWithinLimit 序列化并按降级阶梯把结果压进 MaxRecordBytes（闸1c）：
// 原始曲线 → 事件轨迹明细 → 骨架（清异常点、截断查询串）。
//
// 两级上限的分工：
//   - MaxRecordBytes 是可配置的**软预算**，降级到事件骨架为止就不再往下压——轨迹回答
//     「事件为什么没发出去」，比省这点空间更接近排障结论，所以骨架超预算也照写。
//   - maxRecordLineBytes 是由读取端扫描能力反推的**硬上限**，任何路径都不得逾越。
//
// 序列化本身失败时同样走降级重试而不是直接丢：整条记录里的漏斗计数与周期级错误
// 通常比某一条曲线更有排障价值。
func (w *Writer) marshalWithinLimit(r *EvalRecord) ([]byte, bool) {
	line, err := json.Marshal(r)
	if err != nil {
		logger.Warningf("evallog marshal rule %d error: %v, retry as summary", r.RuleId, err)
		r.shrinkToSkeleton()
		r.stripEventDetails()
		if line, err = json.Marshal(r); err != nil {
			w.dropRecord(r, "marshal", err)
			return nil, false
		}
		return w.enforceLineCeiling(r, line)
	}

	if len(line) > w.cfg.MaxRecordBytes {
		r.stripSeries()
		if line, err = json.Marshal(r); err != nil {
			w.dropRecord(r, "marshal after strip series", err)
			return nil, false
		}
	}
	if len(line) > w.cfg.MaxRecordBytes {
		r.stripEventDetails()
		if line, err = json.Marshal(r); err != nil {
			w.dropRecord(r, "marshal after strip event details", err)
			return nil, false
		}
	}
	if len(line) > w.cfg.MaxRecordBytes {
		// 前两级只动 Series 与事件明细，Query / Error / Warnings / Anomalies.Key 这些
		// 可变长字段不受影响，光靠它们收敛不了，这一级把它们也压掉
		r.shrinkToSkeleton()
		if line, err = json.Marshal(r); err != nil {
			w.dropRecord(r, "marshal after shrink", err)
			return nil, false
		}
	}
	return w.enforceLineCeiling(r, line)
}

// enforceLineCeiling 保证写出的行不超过读取端能扫描的长度，必要时逐步砍掉事件轨迹。
// 超过该长度的行会让 scanJsonlFile 的 bufio.Scanner 中止，该小时文件**从这一行起**的
// 所有记录都读不到——损失远大于丢掉这一条，所以这里宁可丢。
func (w *Writer) enforceLineCeiling(r *EvalRecord, line []byte) ([]byte, bool) {
	for len(line) > maxRecordLineBytes && len(r.Events) > 0 {
		r.Events = r.Events[:len(r.Events)/2]
		r.Truncated = true
		var err error
		if line, err = json.Marshal(r); err != nil {
			w.dropRecord(r, "marshal after trimming events", err)
			return nil, false
		}
	}
	if len(line) > maxRecordLineBytes {
		w.dropRecord(r, fmt.Sprintf("record is %d bytes after full degrade, exceeds line ceiling %d", len(line), maxRecordLineBytes), nil)
		return nil, false
	}
	return line, true
}

// dropRecord 统一记录「这一条最终没能落盘」，并计入丢弃指标，避免静默消失。
func (w *Writer) dropRecord(r *EvalRecord, reason string, err error) {
	if err != nil {
		logger.Warningf("evallog drop record rule %d ds %d: %s: %v", r.RuleId, r.DatasourceId, reason, err)
	} else {
		logger.Warningf("evallog drop record rule %d ds %d: %s", r.RuleId, r.DatasourceId, reason)
	}
	if w.dropHook != nil {
		w.dropHook()
	}
}

// budgetFor 取出（必要时创建）某规则（key = rule_id）的当日预算计数，跨日自动清零。
func (w *Writer) budgetFor(key, date string, now time.Time) *ruleBudget {
	b, ok := w.budgets[key]
	if !ok {
		b = &ruleBudget{date: date}
		w.budgets[key] = b
	}
	if b.date != date {
		b.date = date
		b.bytes = 0
	}
	b.lastUse = now
	return b
}

// getAppender 取出/创建目标小时文件的 appender，必要时滚动旧文件。
func (w *Writer) getAppender(key string, r *EvalRecord) *appender {
	t := msTime(r.Ts)
	hourKey := t.Format(dateLayout) + "/" + t.Format(hourLayout)

	a, ok := w.appenders[key]
	if ok && a.hourKey == hourKey {
		return a
	}

	if ok {
		// 跨小时：关闭旧文件并压缩
		w.finalize(a, true)
	} else if len(w.appenders) >= maxOpenAppenders {
		w.evictOldest()
	}

	dir := filepath.Join(w.cfg.Dir, key, t.Format(dateLayout))
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Warningf("evallog mkdir %s error: %v", dir, err)
		return nil
	}
	path := filepath.Join(dir, t.Format(hourLayout)+".jsonl")
	// 该路径可能正被异步 gzip 处理（迟到的跨整点记录）。必须等它压完再打开，
	// 否则拿到的是马上会被 os.Remove 掉的 inode，之后的写入静默丢失。
	w.waitGzDone(path)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if os.IsNotExist(err) {
		// 清理协程的 pruneEmptyDirs 可能刚好在 MkdirAll 与 OpenFile 之间把这个空目录删掉
		// （写入历史小时的记录时尤其可能命中），重建一次再试
		if mkErr := os.MkdirAll(dir, 0755); mkErr == nil {
			f, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		}
	}
	if err != nil {
		logger.Warningf("evallog open %s error: %v", path, err)
		return nil
	}

	na := &appender{
		ruleDs:   key,
		hourPath: path,
		hourKey:  hourKey,
		f:        f,
		bw:       bufio.NewWriterSize(f, 64*1024),
	}
	w.appenders[key] = na
	return na
}

// finalize 刷盘并关闭 appender；compress=true 时投递异步 gzip。
func (w *Writer) finalize(a *appender, compress bool) {
	if err := a.bw.Flush(); err != nil {
		logger.Warningf("evallog flush %s error: %v", a.hourPath, err)
	}
	a.f.Close()
	delete(w.appenders, a.ruleDs)
	if compress {
		w.enqueueGz(a.hourPath)
	}
}

// housekeep 周期任务：刷盘、滚动已跨小时的文件、关闭空闲句柄。
func (w *Writer) housekeep() {
	now := time.Now()

	// 回收长期无写入的预算计数，避免规则大量增删后 budgets 无界增长
	for k, b := range w.budgets {
		if now.Sub(b.lastUse) > budgetIdleTimeout {
			delete(w.budgets, k)
		}
	}

	nowHourKey := now.Format(dateLayout) + "/" + now.Format(hourLayout)
	for _, a := range w.appenders {
		// 熔断期内不再做周期刷盘/滚动：尽力刷一次就收口句柄，能刷出去的保住，
		// 刷不出去的损失已由熔断的日志与计数覆盖，不再逐秒逐句柄重复报错。
		// 每轮重读熔断态：本循环内 Flush 失败进入熔断后，其余句柄立刻走这条收口路径。
		if w.degraded(now) {
			a.bw.Flush()
			a.f.Close()
			delete(w.appenders, a.ruleDs)
			continue
		}
		if a.hourKey != nowHourKey {
			// 该小时已过且不再有写入进来，滚动压缩
			w.finalize(a, true)
			continue
		}
		if time.Since(a.lastWrite) > appenderIdleTimeout {
			// 空闲关闭；若小时已过（含边界竞争）交给下轮 hourKey 分支处理，这里只处理当前小时
			w.finalize(a, false)
			continue
		}
		if err := a.bw.Flush(); err != nil {
			logger.Warningf("evallog flush %s error: %v", a.hourPath, err)
			// Flush 失败与写失败同源（盘级故障）：收口句柄并进入熔断，
			// 否则粘滞在 bufio 上的 error 会让之后每一秒、每个句柄都重复上面那条 Warning
			a.f.Close()
			delete(w.appenders, a.ruleDs)
			w.markDegraded(now, "flush failed")
		}
	}
}

func (w *Writer) closeAll(compress bool) {
	for _, a := range w.appenders {
		w.finalize(a, compress)
	}
}

func (w *Writer) evictOldest() {
	var oldest *appender
	for _, a := range w.appenders {
		if oldest == nil || a.lastWrite.Before(oldest.lastWrite) {
			oldest = a
		}
	}
	if oldest != nil {
		nowHourKey := time.Now().Format(dateLayout) + "/" + time.Now().Format(hourLayout)
		w.finalize(oldest, oldest.hourKey != nowHourKey)
	}
}

func (w *Writer) enqueueGz(path string) {
	w.gzMu.Lock()
	defer w.gzMu.Unlock()
	if w.gzClosed {
		return
	}
	if _, busy := w.gzInflight[path]; busy {
		// 同一路径已在队列里，重复投递只会让 gzLoop 对着同一个文件白跑一次
		return
	}
	done := make(chan struct{})
	w.gzInflight[path] = done
	w.gzWg.Add(1)
	select {
	case w.gzCh <- path:
	default:
		// 没能入队就必须立刻撤销登记并放行等待者，否则 waitGzDone 会永远阻塞消费 goroutine
		delete(w.gzInflight, path)
		close(done)
		w.gzWg.Done()
		logger.Warningf("evallog gzip queue full, skip compress %s", path)
	}
}

func (w *Writer) gzLoop() {
	defer w.gzWg.Done()
	for path := range w.gzCh {
		if err := compressFile(path); err != nil && !os.IsNotExist(err) {
			logger.Warningf("evallog compress %s error: %v", path, err)
		}
		w.finishGz(path)
		w.gzWg.Done()
	}
}

// finishGz 撤销登记并唤醒等待该路径的消费 goroutine。
func (w *Writer) finishGz(path string) {
	w.gzMu.Lock()
	done := w.gzInflight[path]
	delete(w.gzInflight, path)
	w.gzMu.Unlock()
	if done != nil {
		close(done)
	}
}

// waitGzDone 等待该路径上排队/进行中的压缩任务结束。
//
// compressFile 成功后会 os.Remove(path)。若在压缩进行中重新打开同名文件，新 appender
// 持有的就是一个随即被 unlink 的 inode：后续写入不报错，却永远读不回来；该 appender
// 下次滚动时 compressFile 又拿到 ENOENT 被 gzLoop 的 os.IsNotExist 分支吞掉，全程无日志。
// 触发路径是跨整点的慢评估（Ts 取评估开始时刻、Push 在评估结束时执行）——整点后
// housekeep 已 finalize 并投递压缩，迟到记录随后到达，getAppender 就会重建同名 .jsonl。
//
// 只有历史小时文件会被投递压缩，所以这里只在迟到记录这条冷路径上真正阻塞；
// gzLoop 不会反向等待消费 goroutine，不存在环路。
func (w *Writer) waitGzDone(path string) {
	w.gzMu.Lock()
	done, busy := w.gzInflight[path]
	w.gzMu.Unlock()
	if !busy {
		return
	}
	<-done
}

// sweepLeftovers 启动时把历史遗留的非当前小时 .jsonl 压缩掉（上次进程退出/崩溃残留）。
//
// 只处理本包自己写出的 {rule_id}_{ds_id}/{date}/{hour}.jsonl：命中的文件会被压成 .gz
// 并**删除原文件**（compressFile 末尾的 os.Remove），而 Dir 是运维可配项。只按 .jsonl
// 后缀遍历整棵子树，Dir 一旦误配成某个已有目录，该目录下所有非当前小时的 .jsonl 都会
// 被改写并删除——这正是 cleaner 用 ownHourFile 守住的那条线，写入侧同样不能少。
func (w *Writer) sweepLeftovers() {
	currentHour := truncHour(time.Now())
	_ = filepath.Walk(w.cfg.Dir, func(path string, info os.FileInfo, err error) error {
		if w.gzStopped() {
			// Writer 已关闭（进程退出 / 重新 Init），继续遍历已无意义
			return filepath.SkipAll
		}
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		h, ok := ownHourFile(w.cfg.Dir, path)
		if !ok {
			return nil
		}
		if !h.Before(currentHour) {
			return nil // 当前（及未来）小时文件继续追加
		}
		w.enqueueGz(path)
		return nil
	})
}

func compressFile(path string) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()

	dst := path + ".gz"

	// 同一小时可能被滚动压缩两次：整点后 housekeep 已经把该小时压成 .gz，随后一个
	// 跨整点的慢评估（Ts 取评估开始时刻、Push 在评估结束时执行）又带着上一小时的 Ts
	// 到达，getAppender 会重建同名 .jsonl。此时若仍走 rename，就会把整小时数据的 .gz
	// 整份替换成只含这一两条迟到记录的新文件。
	// gzip 成员可拼接、gzip.Reader 默认 multistream，所以目标已存在时追加而不是覆盖。
	if _, statErr := os.Stat(dst); statErr == nil {
		return appendCompressed(dst, in, path)
	} else if !os.IsNotExist(statErr) {
		return statErr
	}

	tmp := path + ".gz.tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}

	zw := gzip.NewWriter(out)
	if _, err = io.Copy(zw, in); err == nil {
		err = zw.Close()
	} else {
		zw.Close()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}

	if err = os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Remove(path)
}

// appendCompressed 把 src 的内容压成一个新的 gzip 成员追加到已存在的 dst 末尾，
// 成功后删除 src。追加中途失败时 dst 末尾会留下一个不完整成员，读取端
// （scanJsonlFile 遇到 scanner.Err 时保留已扫到的部分）仍能拿到此前的全部记录，
// 比原先整份覆盖丢掉一小时数据要好得多。
func appendCompressed(dst string, in io.Reader, src string) error {
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	zw := gzip.NewWriter(out)
	_, err = io.Copy(zw, in)
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Remove(src)
}

func msTime(ms int64) time.Time {
	return time.Unix(ms/1000, (ms%1000)*int64(time.Millisecond))
}
