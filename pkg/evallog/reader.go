package evallog

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/toolkits/pkg/logger"
)

const (
	// DefaultQueryLimit 查询默认返回条数
	DefaultQueryLimit = 500
	// MaxQueryLimit 查询单次最大返回条数
	MaxQueryLimit = 2000

	// maxScanLineBytes 读取单行 JSONL 的扫描缓冲上限。
	// 写入侧的 maxRecordLineBytes 由它反推，两者必须成对修改。
	maxScanLineBytes = 4 * 1024 * 1024
	// maxRecordLineBytes 写入单行的硬上限，留出换行与编码余量
	maxRecordLineBytes = maxScanLineBytes - 1024
)

// truncHour 按**本地时区**把时间截到整点。
//
// 不能用 time.Truncate(time.Hour)：它按自零时刻起的绝对时长取整，不作用于展现形式，
// 而写入侧是按本地展现形式分桶的（t.Format("2006-01-02") + "/" + t.Format("15")）。
// 在 +05:30 / +05:45 / +03:30 / +09:30 / -03:30 这类非整点偏移的时区里，Truncate 的
// 结果会落在本地的 :30 或 :45 上，Format("15") 拿到的是上一个小时，于是每小时前半段
// 的记录永远定位不到文件——排障时会被读成「当时确实没数据」。
func truncHour(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
}

// clampFromToRetention 把查询下界钳到保留期起点。
//
// 否则 from=0（引擎侧 handler 的历史默认值，也可由调用方显式传入）会让小时循环从
// 当前时刻一路倒退到 1970 年——约 49 万次迭代、近百万次 os.Open，单次请求即可占住
// handler 数秒，反复发起就是对告警引擎的慢速 DoS。保留期之外本就没有数据可读，
// 钳掉不损失任何结果。
func clampFromToRetention(cfg Config, fromMs int64, now time.Time) int64 {
	floor := now.Add(-time.Duration(cfg.RetentionHours) * time.Hour).UnixMilli()
	if fromMs < floor {
		return floor
	}
	return fromMs
}

// queryRecords 扫描 [fromMs, toMs] 覆盖的小时文件，返回按 ts 倒序的记录。
// 小时文件按文件名直接定位，无需索引；beforeMs > 0 时作为翻页游标（ts < beforeMs）。
func queryRecords(cfg Config, ruleId, datasourceId int64, fromMs, toMs, beforeMs int64, limit int) ([]EvalRecord, error) {
	if limit <= 0 {
		limit = DefaultQueryLimit
	}
	if limit > MaxQueryLimit {
		limit = MaxQueryLimit
	}
	if toMs <= 0 {
		toMs = time.Now().UnixMilli()
	}
	if fromMs < 0 || fromMs > toMs {
		return nil, fmt.Errorf("invalid time range: from %d > to %d", fromMs, toMs)
	}
	fromMs = clampFromToRetention(cfg, fromMs, time.Now())
	if fromMs > toMs {
		return []EvalRecord{}, nil
	}
	upper := toMs
	if beforeMs > 0 && beforeMs-1 < upper {
		upper = beforeMs - 1
	}
	if upper < fromMs {
		return []EvalRecord{}, nil
	}

	ruleDir := filepath.Join(cfg.Dir, fmt.Sprintf("%d_%d", ruleId, datasourceId))
	if _, err := os.Stat(ruleDir); os.IsNotExist(err) {
		return []EvalRecord{}, nil
	}

	result := make([]EvalRecord, 0, 64)
	// 从最新小时向旧遍历，攒够 limit 即止
	for h := truncHour(msTime(upper)); !h.Before(truncHour(msTime(fromMs))); h = h.Add(-time.Hour) {
		remaining := limit - len(result)
		if remaining <= 0 {
			break
		}
		// 把剩余 limit 下推到扫描环节：每个小时只留该小时内最新的 remaining 条，
		// 常驻内存由 limit 决定，而不是由小时文件大小决定
		ring := newRecordRing(remaining)
		if err := scanHourFile(ruleDir, h, fromMs, upper, ring); err != nil {
			return nil, err
		}
		result = ring.appendDesc(result)
	}
	return result, nil
}

// tsProbe 只解出 ts 字段。区间外的行占绝大多数，对它们做整条反序列化纯属浪费。
type tsProbe struct {
	Ts int64 `json:"ts"`
}

// recordRing 定长环形缓冲，保留扫描过程中最后 cap 条命中记录。
//
// 小时文件内是时间升序，"最后 N 条"就是该小时里最新的 N 条，正是倒序结果需要的那段。
// 之所以不能像原先那样先把整个文件读成切片再截取：单条记录硬上限接近 4MB、
// PerRuleDailyMB 默认允许每规则每天写 1GB，单个小时文件可达数十 MB，解码成结构体后
// 还要再放大几倍，一次合法查询就能在告警引擎上造成数百 MB 级堆分配。
type recordRing struct {
	buf   []EvalRecord
	cap   int
	start int // 最旧元素下标
	size  int
}

// newRecordRing 惰性分配：一次查询最多要扫 RetentionHours 个小时，其中绝大多数小时
// 根本没有文件，每轮都按 limit 预分配底层数组只是白白制造垃圾。
func newRecordRing(capacity int) *recordRing {
	if capacity < 1 {
		capacity = 1
	}
	return &recordRing{cap: capacity}
}

func (r *recordRing) push(rec EvalRecord) {
	if len(r.buf) < r.cap {
		r.buf = append(r.buf, rec)
		r.size = len(r.buf)
		return
	}
	// 满了：覆盖最旧的一条并前移起点
	r.buf[r.start] = rec
	r.start = (r.start + 1) % len(r.buf)
}

// appendDesc 按时间倒序（最新在前）把环内元素追加到 dst。
func (r *recordRing) appendDesc(dst []EvalRecord) []EvalRecord {
	if r.size == 0 {
		return dst
	}
	for i := r.size - 1; i >= 0; i-- {
		dst = append(dst, r.buf[(r.start+i)%len(r.buf)])
	}
	return dst
}

// scanHourFile 流式扫描某小时的记录文件，把 [fromMs, upperMs] 内的记录喂给 ring。
func scanHourFile(ruleDir string, hour time.Time, fromMs, upperMs int64, ring *recordRing) error {
	base := filepath.Join(ruleDir, hour.Format(dateLayout), hour.Format(hourLayout))

	// 顺序固定为 .gz 在前、.jsonl 在后：滚动压缩与写入存在短暂共存窗口，两个文件都要读，
	// 且 .gz 里的记录更早，先扫才能让 ring 内保持时间升序。
	for _, f := range []struct {
		path    string
		gzipped bool
	}{
		{base + ".jsonl.gz", true},
		{base + ".jsonl", false},
	} {
		if err := scanJsonlFile(f.path, f.gzipped, fromMs, upperMs, ring); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func scanJsonlFile(path string, gzipped bool, fromMs, upperMs int64, ring *recordRing) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var r io.Reader = f
	if gzipped {
		zr, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("evallog gzip open %s: %v", path, err)
		}
		defer zr.Close()
		r = zr
	}

	matched := 0
	scanner := bufio.NewScanner(r)
	// 单条记录上限 256KB（默认），buffer 放宽到 maxScanLineBytes 以兼容配置放大的场景；
	// 写入侧的 enforceLineCeiling 保证不会写出超过这个长度的行
	scanner.Buffer(make([]byte, 64*1024), maxScanLineBytes)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var probe tsProbe
		if err := json.Unmarshal(line, &probe); err != nil {
			// 尾部半行（进程崩溃或并发追加中）跳过
			continue
		}
		if probe.Ts < fromMs || probe.Ts > upperMs {
			continue
		}
		var rec EvalRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		ring.push(rec)
		matched++
	}
	if err := scanner.Err(); err != nil {
		// 截断的 gzip 尾、超长行等异常：保留已扫到的部分而不是整体失败，
		// 但必须留下痕迹——静默返回半个文件会被当成「这段时间就这么多记录」
		logger.Warningf("evallog scan %s stopped early after %d matched records: %v", path, matched, err)
	}
	return nil
}
