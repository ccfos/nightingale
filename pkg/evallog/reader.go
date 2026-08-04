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
		recs, err := readHourFile(ruleDir, h)
		if err != nil {
			return nil, err
		}
		// 文件内按写入序（时间升序），倒序过滤
		for i := len(recs) - 1; i >= 0; i-- {
			if recs[i].Ts < fromMs || recs[i].Ts > upper {
				continue
			}
			result = append(result, recs[i])
			if len(result) >= limit {
				return result, nil
			}
		}
	}
	return result, nil
}

// readHourFile 读取某小时的记录文件，优先未压缩文件（当前小时），其次 .gz。
func readHourFile(ruleDir string, hour time.Time) ([]EvalRecord, error) {
	base := filepath.Join(ruleDir, hour.Format(dateLayout), hour.Format(hourLayout))

	if recs, err := readJsonlFile(base+".jsonl", false); err == nil {
		// 滚动压缩与写入存在短暂共存窗口，两个都读并合并
		if gzRecs, gzErr := readJsonlFile(base+".jsonl.gz", true); gzErr == nil {
			return append(gzRecs, recs...), nil
		}
		return recs, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	recs, err := readJsonlFile(base+".jsonl.gz", true)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return recs, err
}

func readJsonlFile(path string, gzipped bool) ([]EvalRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var r io.Reader = f
	if gzipped {
		zr, err := gzip.NewReader(f)
		if err != nil {
			return nil, fmt.Errorf("evallog gzip open %s: %v", path, err)
		}
		defer zr.Close()
		r = zr
	}

	var recs []EvalRecord
	scanner := bufio.NewScanner(r)
	// 单条记录上限 256KB（默认），buffer 放宽到 maxScanLineBytes 以兼容配置放大的场景；
	// 写入侧的 enforceLineCeiling 保证不会写出超过这个长度的行
	scanner.Buffer(make([]byte, 64*1024), maxScanLineBytes)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec EvalRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			// 尾部半行（进程崩溃或并发追加中）跳过
			continue
		}
		recs = append(recs, rec)
	}
	if err := scanner.Err(); err != nil {
		// 截断的 gzip 尾、超长行等异常：返回已解析部分而不是整体失败，
		// 但必须留下痕迹——静默返回半个文件会被当成「这段时间就这么多记录」
		logger.Warningf("evallog scan %s stopped early after %d records: %v", path, len(recs), err)
		return recs, nil
	}
	return recs, nil
}
