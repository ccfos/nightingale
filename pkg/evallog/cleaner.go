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

// cleanInterval 清理周期。不能拉长到小时级：MaxDiskGB 的兜底判定只在每轮清理时发生，
// 间隔多长，目录就能超出预算多久。
const cleanInterval = 10 * time.Minute

// ruleDirPattern / hourFilePattern 限定清理的删除面。
//
// Dir 是运维可配项，而「按日期分子目录」的日志/数据目录极其常见：一旦误配成某个已有
// 目录，不加校验的清理会把 {Dir}/<任意名字>/<YYYY-MM-DD>/ 连同内容递归删掉。只处理
// 本包自己写出的 {rule_id}_{ds_id}/{date}/{hour}.jsonl(.gz)，其余一律不碰。
var (
	ruleDirPattern  = regexp.MustCompile(`^\d+_\d+$`)
	hourFilePattern = regexp.MustCompile(`^\d{2}\.jsonl(\.gz)?$`)
)

// ownHourFile 判定 path 是否是本包写出的小时文件，是则返回它所属的整点时刻。
//
// 凡是会删除或改写文件的入口都必须先过这道校验，不只是 cleaner：writer 启动时的
// sweepLeftovers 同样会把命中的文件压成 .gz 并删除原文件（compressFile 末尾的
// os.Remove），Dir 误配成已有目录时波及面和不加校验的清理完全一样。
func ownHourFile(root, path string) (time.Time, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return time.Time{}, false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) != 3 || !ruleDirPattern.MatchString(parts[0]) {
		return time.Time{}, false
	}
	return parseHourFileName(parts[1], parts[2])
}

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

// parseHourFileName 把日期目录名 + 小时文件名解析为整点时刻，非本包布局返回 false。
func parseHourFileName(dateName, fileName string) (time.Time, bool) {
	if !hourFilePattern.MatchString(fileName) {
		return time.Time{}, false
	}
	name := strings.TrimSuffix(strings.TrimSuffix(fileName, ".gz"), ".jsonl")
	h, err := time.ParseInLocation(dateLayout+" "+hourLayout, dateName+" "+name, time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return h, true
}

func cleanOnce(cfg Config) {
	// 第一遍：流式删过期数据，同时按**小时桶**聚合占用。
	// 刻意不把候选文件逐个攒进切片：万级 rule-ds 的部署有数百万个小时文件，
	// 每轮攒一遍就是数百 MB 的存活堆尖峰，GC 会把它摊到评估延迟上；
	// 而删除严格按小时从旧到新，知道每个小时桶的总字节数就足够决策，
	// 桶数被 RetentionHours 封顶（默认 192 个），内存 O(小时数) 而非 O(文件数)。
	total, buckets := sweepExpiredAndMeasure(cfg)

	budget := int64(cfg.MaxDiskGB) * 1024 * 1024 * 1024
	if total > budget {
		pruneToBudget(cfg, buckets, total, budget)
	}

	pruneEmptyDirs(cfg.Dir)
}

// sweepExpiredAndMeasure 按 RetentionHours 边走边删过期数据（天目录/小时文件粒度），
// 返回目录实际占用 total 与可删候选按小时聚合的字节数（key 为整点的 Unix 秒）。
//
// total 与 buckets 口径**不同**：当前整点的文件计入 total 但不进 buckets——
// total 若不含当前小时，预算判定会系统性低估占用（写入几乎全部集中在当前小时），
// 磁盘早已超过 MaxDiskGB 时可能一个旧文件都不删；而当前（及未来）小时的文件
// 不可删：句柄还被 writer 持有，删了既不立刻释放空间，又会让这一小时的记录
// 对查询端消失。
func sweepExpiredAndMeasure(cfg Config) (int64, map[int64]int64) {
	cutoff := time.Now().Add(-time.Duration(cfg.RetentionHours) * time.Hour)
	currentHour := truncHour(time.Now())

	var total int64
	buckets := make(map[int64]int64, cfg.RetentionHours+1)

	ruleDirs, err := os.ReadDir(cfg.Dir)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warningf("evallog clean read dir %s error: %v", cfg.Dir, err)
		}
		return 0, buckets
	}

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
				if hf.IsDir() {
					continue
				}
				h, ok := parseHourFileName(dd.Name(), hf.Name())
				if !ok {
					continue
				}
				// 边界天内早于 cutoff 的小时文件
				if h.Add(time.Hour).Before(cutoff) {
					path := filepath.Join(datePath, hf.Name())
					if err := os.Remove(path); err != nil {
						logger.Warningf("evallog clean remove %s error: %v", path, err)
					}
					continue
				}
				info, err := hf.Info()
				if err != nil {
					continue
				}
				total += info.Size()
				if !h.Before(currentHour) {
					continue
				}
				buckets[h.Unix()] += info.Size()
			}
		}
	}
	return total, buckets
}

// pruneToBudget 超出 MaxDiskGB 时的兜底：按小时从旧到新删，直到回到预算内。
//
// 先在 ≤RetentionHours 个小时桶上算出分界：分界之前的小时整桶删，分界小时只删到
// 腾够为止；再走一遍目录执行。第二遍扫描的代价只在超预算这个异常态才发生，
// 换来第一遍的存活内存从 O(文件数) 降到 O(小时数)。
func pruneToBudget(cfg Config, buckets map[int64]int64, total, budget int64) {
	hours := make([]int64, 0, len(buckets))
	for h := range buckets {
		hours = append(hours, h)
	}
	if len(hours) == 0 {
		// 没有任何可删候选：超出的全是当前小时（句柄未释放，不能删）。
		// 这条日志是 MaxDiskGB 已经压不住写入速率的信号，不能和正常清理混为一谈。
		logger.Warningf("evallog clean: still %d bytes after pruning all closed hour files, budget %d; "+
			"lower PerRuleDailyMB / MaxSeriesPerQuery or raise MaxDiskGB", total, budget)
		return
	}
	sort.Slice(hours, func(i, j int) bool { return hours[i] < hours[j] })

	// boundary：需要动到的最新小时；fromBoundary：分界小时里需要腾出的字节数。
	// 候选全删仍不够时 boundary 停在最新候选小时、fromBoundary 取整桶。
	remain := total - budget
	boundary := hours[0]
	var fromBoundary int64
	for _, h := range hours {
		boundary = h
		fromBoundary = buckets[h]
		if remain <= buckets[h] {
			fromBoundary = remain
			remain = 0
			break
		}
		remain -= buckets[h]
	}

	var freed, freedFromBoundary int64
	ruleDirs, err := os.ReadDir(cfg.Dir)
	if err != nil {
		return
	}
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
			datePath := filepath.Join(rulePath, dd.Name())
			hourFiles, err := os.ReadDir(datePath)
			if err != nil {
				continue
			}
			for _, hf := range hourFiles {
				if hf.IsDir() {
					continue
				}
				h, ok := parseHourFileName(dd.Name(), hf.Name())
				if !ok {
					continue
				}
				hu := h.Unix()
				// 分界之后的小时（含当前/未来小时）不动；分界小时腾够即止
				if hu > boundary || (hu == boundary && freedFromBoundary >= fromBoundary) {
					continue
				}
				info, err := hf.Info()
				if err != nil {
					continue
				}
				path := filepath.Join(datePath, hf.Name())
				if err := os.Remove(path); err != nil {
					logger.Warningf("evallog clean remove %s error: %v", path, err)
					continue
				}
				freed += info.Size()
				if hu == boundary {
					freedFromBoundary += info.Size()
				}
			}
		}
	}

	if remain > 0 {
		// 候选删光仍超预算：剩下的都是当前小时，见上方同款日志的注释
		logger.Warningf("evallog clean: still %d bytes after pruning all closed hour files, budget %d; "+
			"lower PerRuleDailyMB / MaxSeriesPerQuery or raise MaxDiskGB", total-freed, budget)
		return
	}
	logger.Infof("evallog clean: disk budget exceeded, pruned to %d bytes", total-freed)
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
