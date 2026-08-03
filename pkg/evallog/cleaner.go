package evallog

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"
)

const cleanInterval = time.Hour

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

	for _, rd := range ruleDirs {
		if !rd.IsDir() {
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
				if hf.IsDir() {
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
func pruneEmptyDirs(root string) {
	ruleDirs, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, rd := range ruleDirs {
		if !rd.IsDir() {
			continue
		}
		rulePath := filepath.Join(root, rd.Name())
		dateDirs, _ := os.ReadDir(rulePath)
		for _, dd := range dateDirs {
			if !dd.IsDir() {
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
