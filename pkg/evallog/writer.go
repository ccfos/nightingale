package evallog

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
)

// Writer 单 goroutine 消费队列、按 {rule_ds}/{date}/{hour}.jsonl 追加写入，
// 整点滚动后异步 gzip。所有 appender 状态仅在消费 goroutine 内访问，无锁。
type Writer struct {
	cfg      Config
	ch       chan *EvalRecord
	done     chan struct{} // 消费 goroutine 退出信号
	closing  sync.Once
	dropHook func()

	appenders map[string]*appender // key: {rule_id}_{ds_id}

	gzCh chan string
	gzWg sync.WaitGroup
}

type appender struct {
	ruleDs    string
	hourPath  string // 当前打开文件的完整路径
	hourKey   string // "2006-01-02/15"
	f         *os.File
	bw        *bufio.Writer
	lastWrite time.Time

	// 闸2：当日写入预算
	budgetDate  string
	budgetBytes int64
}

func NewWriter(cfg Config, dropHook func()) (*Writer, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("evallog dir is blank")
	}
	if err := os.MkdirAll(cfg.Dir, 0755); err != nil {
		return nil, fmt.Errorf("evallog mkdir %s: %v", cfg.Dir, err)
	}

	w := &Writer{
		cfg:       cfg,
		ch:        make(chan *EvalRecord, cfg.QueueSize),
		done:      make(chan struct{}),
		dropHook:  dropHook,
		appenders: make(map[string]*appender),
		gzCh:      make(chan string, 1024),
	}

	w.gzWg.Add(1)
	go w.gzLoop()
	go w.loop()
	go w.sweepLeftovers()
	return w, nil
}

// Push 非阻塞入队，队列满则丢弃并回调 dropHook。
func (w *Writer) Push(r *EvalRecord) {
	// Close 与并发 Push 竞争时向已关闭 channel 发送会 panic，吞掉即可：
	// 关闭发生在进程退出/重新 Init，丢这一条记录无影响
	defer func() { _ = recover() }()
	select {
	case w.ch <- r:
	default:
		if w.dropHook != nil {
			w.dropHook()
		}
	}
}

// Close 停止接收、清空队列、刷盘并等待压缩任务完成。
func (w *Writer) Close() {
	w.closing.Do(func() {
		close(w.ch)
		<-w.done
		close(w.gzCh)
		w.gzWg.Wait()
	})
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
	line, err := json.Marshal(r)
	if err != nil {
		logger.Warningf("evallog marshal rule %d error: %v", r.RuleId, err)
		return
	}

	// 闸1c：单条记录硬上限。降级阶梯：先丢原始曲线，仍超限再把事件轨迹压到 hash+stage 骨架
	if len(line) > w.cfg.MaxRecordBytes {
		r.stripSeries()
		if line, err = json.Marshal(r); err != nil {
			return
		}
		if len(line) > w.cfg.MaxRecordBytes {
			r.stripEventDetails()
			if line, err = json.Marshal(r); err != nil {
				return
			}
		}
	}

	a := w.getAppender(r)
	if a == nil {
		return
	}

	// 闸2：单规则单日预算，超出后降级为摘要模式
	date := msTime(r.Ts).Format(dateLayout)
	if a.budgetDate != date {
		a.budgetDate = date
		a.budgetBytes = 0
	}
	// 超预算后降级为摘要模式：同样先丢曲线，事件轨迹保留骨架
	limit := int64(w.cfg.PerRuleDailyMB) * 1024 * 1024
	if a.budgetBytes+int64(len(line)) > limit && (len(r.Queries) > 0 || len(r.Events) > 0) {
		r.stripSeries()
		r.stripEventDetails()
		if line, err = json.Marshal(r); err != nil {
			return
		}
	}
	a.budgetBytes += int64(len(line))

	if _, err = a.bw.Write(append(line, '\n')); err != nil {
		logger.Warningf("evallog write %s error: %v", a.hourPath, err)
	}
	a.lastWrite = time.Now()
}

// getAppender 取出/创建目标小时文件的 appender，必要时滚动旧文件。
func (w *Writer) getAppender(r *EvalRecord) *appender {
	key := fmt.Sprintf("%d_%d", r.RuleId, r.DatasourceId)
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
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
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
	if ok {
		// 保留跨小时前累计的预算计数
		na.budgetDate = a.budgetDate
		na.budgetBytes = a.budgetBytes
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
	nowHourKey := time.Now().Format(dateLayout) + "/" + time.Now().Format(hourLayout)
	for _, a := range w.appenders {
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
	w.gzWg.Add(1)
	select {
	case w.gzCh <- path:
	default:
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
		w.gzWg.Done()
	}
}

// sweepLeftovers 启动时把历史遗留的非当前小时 .jsonl 压缩掉（上次进程退出/崩溃残留）。
func (w *Writer) sweepLeftovers() {
	nowPathSuffix := filepath.Join(time.Now().Format(dateLayout), time.Now().Format(hourLayout)+".jsonl")
	_ = filepath.Walk(w.cfg.Dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		if strings.HasSuffix(path, nowPathSuffix) {
			return nil // 当前小时文件继续追加
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

	if err = os.Rename(tmp, path+".gz"); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Remove(path)
}

func msTime(ms int64) time.Time {
	return time.Unix(ms/1000, (ms%1000)*int64(time.Millisecond))
}
