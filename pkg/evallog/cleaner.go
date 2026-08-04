package evallog

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"
)

const cleanInterval = time.Hour

// ruleDirPattern / hourFilePattern 限定清理的删除面。
//
// Dir 是运维可配项，而「按日期分子目录」的日志/数据目录极其常见：一旦误配成某个已有
// 目录，不加校验的清理会把 {Dir}/<任意名字>/<YYYY-MM-DD>/ 连同内容递归删掉。只处理
// 本包自己写出的 {rule_id}_{ds_id}/{date}/{hour}.jsonl(.gz)，其余一律不碰。
var (
	ruleDirPattern  = regexp.MustCompile(`^\d+_\d+$`)
	hourFilePattern = regexp.MustCompile(`^\d{2}\.jsonl(\.gz)?$`)
)

// cleanLoop 周期清理：按 RetentionHours 删过期数据（天目录/小时文件粒度），
// 再按 MaxDiskGB 兜底从最旧小时删起。
func cleanLoop(cfg Config, stop chan struct{}) {
	cleanOnce(cfg)
	ticker := time.NewTicker(cleanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			cleanOnce(cfg)
		}
	}
}

type hourFile struct {
	path string
	hour time.Time
	size int64
}

func cleanOnce(cfg Config) {
	cutoff := time.Now().Add(-time.Duration(cfg.RetentionHours) * time.Hour)

	ruleDirs, err := os.ReadDir(cfg.Dir)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warningf("evallog clean read dir %s error: %v", cfg.Dir, err)
		}
		return
	}

	var files []hourFile
	var total int64
	// 当前整点的文件仍被 writer 持有句柄并追加，不能进入总量兜底的候选：
	// 删掉它既不会立刻释放空间（fd 还持有 inode），又会让这一小时的记录对查询端消失
	currentHour := truncHour(time.Now())

	for _, rd := range ruleDirs {
		if !rd.IsDir() || !ruleDirPattern.MatchString(rd.Name()) {
			continue
		}
		rulePath := filepath.Join(cfg.Dir, rd.Name())
		dateDirs, err := os.ReadDir(rulePath)
		if err != nil {
			continue
		}
		for _, dd := range dateDirs {
			if !dd.IsDir() {
				continue
			}
			date, err := time.ParseInLocation(dateLayout, dd.Name(), time.Local)
			if err != nil {
				continue
			}
			datePath := filepath.Join(rulePath, dd.Name())

			// 整天早于 cutoff：直接删天目录
			if date.AddDate(0, 0, 1).Add(time.Hour).Before(cutoff) {
				if err := os.RemoveAll(datePath); err != nil {
					logger.Warningf("evallog clean remove %s error: %v", datePath, err)
				}
				continue
			}

			hourFiles, err := os.ReadDir(datePath)
			if err != nil {
				continue
			}
			for _, hf := range hourFiles {
				if hf.IsDir() || !hourFilePattern.MatchString(hf.Name()) {
					continue
				}
				name := strings.TrimSuffix(strings.TrimSuffix(hf.Name(), ".gz"), ".jsonl")
				h, err := time.ParseInLocation(dateLayout+" "+hourLayout, dd.Name()+" "+name, time.Local)
				if err != nil {
					continue
				}
				path := filepath.Join(datePath, hf.Name())
				// 边界天内早于 cutoff 的小时文件
				if h.Add(time.Hour).Before(cutoff) {
					if err := os.Remove(path); err != nil {
						logger.Warningf("evallog clean remove %s error: %v", path, err)
					}
					continue
				}
				if !h.Before(currentHour) {
					continue // 当前（及未来）小时的文件不参与总量兜底删除
				}
				info, err := hf.Info()
				if err != nil {
					continue
				}
				files = append(files, hourFile{path: path, hour: h, size: info.Size()})
				total += info.Size()
			}
		}
	}

	// 总量兜底：从最旧小时删起
	pruneToBudget(files, total, int64(cfg.MaxDiskGB)*1024*1024*1024)

	pruneEmptyDirs(cfg.Dir)
}

func pruneToBudget(files []hourFile, total, budget int64) {
	if total <= budget {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].hour.Before(files[j].hour) })
	for _, f := range files {
		if total <= budget {
			break
		}
		if err := os.Remove(f.path); err != nil {
			logger.Warningf("evallog clean remove %s error: %v", f.path, err)
			continue
		}
		total -= f.size
	}
	logger.Infof("evallog clean: disk budget exceeded, pruned to %d bytes", total)
}

// pruneEmptyDirs 删除清理后残留的空天目录/空规则目录。
//
// 跳过今天的天目录：writer 的 getAppender 是「MkdirAll 之后再 OpenFile」两步，
// 中间有窗口。把它刚建好、还没来得及写入的空目录删掉，会让这一轮的 OpenFile 拿到
// ENOENT，记录直接落不了盘。今天的目录本来也马上会被写满，不值得为它冒这个险。
func pruneEmptyDirs(root string) {
	today := time.Now().Format(dateLayout)
	ruleDirs, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, rd := range ruleDirs {
		if !rd.IsDir() || !ruleDirPattern.MatchString(rd.Name()) {
			continue
		}
		rulePath := filepath.Join(root, rd.Name())
		dateDirs, _ := os.ReadDir(rulePath)
		for _, dd := range dateDirs {
			if !dd.IsDir() || dd.Name() == today {
				continue
			}
			datePath := filepath.Join(rulePath, dd.Name())
			if entries, err := os.ReadDir(datePath); err == nil && len(entries) == 0 {
				os.Remove(datePath)
			}
		}
		if entries, err := os.ReadDir(rulePath); err == nil && len(entries) == 0 {
			os.Remove(rulePath)
		}
	}
}
