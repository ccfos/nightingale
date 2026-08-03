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
)

const (
	// DefaultQueryLimit 查询默认返回条数
	DefaultQueryLimit = 500
	// MaxQueryLimit 查询单次最大返回条数
	MaxQueryLimit = 2000
)

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
	for h := msTime(upper).Truncate(time.Hour); !h.Before(msTime(fromMs).Truncate(time.Hour)); h = h.Add(-time.Hour) {
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
	// 单条记录上限 256KB（默认），buffer 放宽到 4MB 以兼容配置放大的场景
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
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
		return recs, nil // 截断的 gzip 尾等异常，返回已解析部分
	}
	return recs, nil
}
