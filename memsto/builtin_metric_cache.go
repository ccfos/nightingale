package memsto

import (
	"encoding/json"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/container/set"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/runner"

	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

const SYSTEM = "system"

type BuiltinMetricCacheType struct {
	statTotal              int64
	statLastUpdated        int64
	ctx                    *ctx.Context
	stats                  *Stats
	builtinIntegrationsDir string // path to the directory containing builtin components, e.g., "/path/to/builtin/components"

	sync.RWMutex
	bm              map[int64]*models.BuiltinMetric // key: id
	expressionIdMap map[string]int64                // key: expression, value: id
}

func NewBuiltinMetricCache(ctx *ctx.Context, stats *Stats, builtinIntegrationsDir string) *BuiltinMetricCacheType {
	bm := &BuiltinMetricCacheType{
		statTotal:              -1,
		statLastUpdated:        -1,
		ctx:                    ctx,
		stats:                  stats,
		builtinIntegrationsDir: builtinIntegrationsDir,
		bm:                     make(map[int64]*models.BuiltinMetric),
		expressionIdMap:        make(map[string]int64),
	}

	bm.SyncBuiltinMetrics()
	return bm
}
func (b *BuiltinMetricCacheType) StatChanged(total, lastUpdated int64) bool {
	if b.statTotal == total && b.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (b *BuiltinMetricCacheType) SyncBuiltinMetrics() {
	b.initBuiltinMetricFiles()

	err := b.syncBuiltinMetrics()
	if err != nil {
		logger.Errorf("failed to sync builtin components: %v", err)
	}

	go b.loopSyncBuiltinMetrics()
}

func (b *BuiltinMetricCacheType) initBuiltinMetricFiles() error {
	fp := b.builtinIntegrationsDir
	if fp == "" {
		fp = path.Join(runner.Cwd, "integrations")
	}

	// var fileList []string
	dirList, err := file.DirsUnder(fp)
	if err != nil {
		logger.Warning("read builtin component dir fail ", err)
		return err
	}

	for _, dir := range dirList {
		// components icon
		componentDir := fp + "/" + dir
		component := models.BuiltinComponent{
			Ident: dir,
		}

		// metrics
		files, err := file.FilesUnder(componentDir + "/metrics")
		if err == nil && len(files) > 0 {
			for _, f := range files {
				fp := componentDir + "/metrics/" + f
				bs, err := file.ReadBytes(fp)
				if err != nil {
					logger.Warning("read builtin component metrics file fail", f, err)
					continue
				}

				metrics := []models.BuiltinMetric{}
				err = json.Unmarshal(bs, &metrics)
				if err != nil {
					logger.Warning("parse builtin component metrics file fail", f, err)
					continue
				}

				for _, metric := range metrics {
					if metric.UUID == 0 {
						metric.UUID = time.Now().UnixNano()
					}

					b.addBuiltinMetric(&metric)
				}
			}
		} else if err != nil {
			logger.Warningf("read builtin component metrics dir fail %s %v", component.Ident, err)
		}
	}

	return nil
}

func (b *BuiltinMetricCacheType) loopSyncBuiltinMetrics() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		// Current metric need to be cleaned up before sync
		// to avoid duplicate metrics.
		b.CleanupBuiltinMetricsCache()
		b.initBuiltinMetricFiles()
		if err := b.syncBuiltinMetrics(); err != nil {
			logger.Warning("failed to sync datasources:", err)
		}
	}
}

func (b *BuiltinMetricCacheType) CleanupBuiltinMetricsCache() {
	b.Lock()
	defer b.Unlock()

	// Clear the cache
	b.bm = make(map[int64]*models.BuiltinMetric)
	b.expressionIdMap = make(map[string]int64)
}

func (b *BuiltinMetricCacheType) syncBuiltinMetrics() error {
	start := time.Now()

	stat, err := models.BuiltinMetricStatistics(b.ctx)
	if err != nil {
		dumper.PutSyncRecord("builtin_metrics", start.Unix(), -1, -1, "failed to query statistics: "+err.Error())
		return errors.WithMessage(err, "failed to exec BuiltinMetricStatistics")
	}

	if !b.StatChanged(stat.Total, stat.LastUpdated) {
		b.stats.GaugeCronDuration.WithLabelValues("sync_builtin_metrics").Set(0)
		b.stats.GaugeSyncNumber.WithLabelValues("sync_builtin_metrics").Set(0)
		dumper.PutSyncRecord("builtin_metrics", start.Unix(), -1, -1, "not changed")
		return nil
	}

	bm, err := models.BuiltinMetricGetAll(b.ctx)
	if err != nil {
		dumper.PutSyncRecord("builtin_metrics", start.Unix(), -1, -1, "failed to query records: "+err.Error())
		return errors.WithMessage(err, "failed to call BuiltinMetricGetMap")
	}

	b.Set(bm, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	b.stats.GaugeCronDuration.WithLabelValues("sync_builtin_metrics").Set(float64(ms))
	b.stats.GaugeSyncNumber.WithLabelValues("sync_builtin_metrics").Set(float64(len(bm)))

	logger.Infof("timer: sync builtin components done, cost: %dms, number: %d", ms, len(bm))
	dumper.PutSyncRecord("builtin_metrics", start.Unix(), ms, len(bm), "success")

	return nil
}

func (b *BuiltinMetricCacheType) Set(bm []*models.BuiltinMetric, total, lastUpdated int64) {
	for _, metric := range bm {
		b.addBuiltinMetric(metric)
	}

	// only one goroutine used, so no need lock
	b.statTotal = total
	b.statLastUpdated = lastUpdated
}

func (b *BuiltinMetricCacheType) GetByBuiltinMetricId(id int64) (*models.BuiltinMetric, error) {
	b.RLock()
	defer b.RLock()
	bp, ok := b.bm[id]
	if !ok {
		return nil, errors.New("builtin metric not found")
	}

	return bp, nil
}

func (b *BuiltinMetricCacheType) Len(lang, collector, typ, query, unit string) int {
	b.RLock()
	defer b.RLock()
	return len(b.bm)
}

// New metrics must use this method to add, so that the cache is updated correctly
// with same expression.
func (b *BuiltinMetricCacheType) addBuiltinMetric(bm *models.BuiltinMetric) error {
	b.Lock()
	defer b.Unlock()

	if _, exists := b.bm[bm.ID]; exists {
		return errors.New("builtin component already exists")
	}

	// Merge to existing metric with same expression
	if bm.Lang == "en_US" {
		return b.addOrUpdateBuiltinMetricForLang(bm)
	}

	return b.addNonEnglishMetric(bm)
}

func (b *BuiltinMetricCacheType) addOrUpdateBuiltinMetricForLang(bm *models.BuiltinMetric) error {
	if existingId, ok := b.expressionIdMap[bm.Expression]; ok {
		// Update the existing metric with the new one
		if existingMetric, exists := b.bm[existingId]; exists {
			// Merge translation to current metric
			bm.Translation = mergeTranslations(existingMetric.Translation, bm.Translation)
		}
		// Delete the old metric
		delete(b.bm, existingId)
	}
	// Direct update
	b.bm[bm.ID] = bm
	b.expressionIdMap[bm.Expression] = bm.ID
	b.statTotal++
	b.statLastUpdated = time.Now().Unix()
	return nil
}

func (b *BuiltinMetricCacheType) addNonEnglishMetric(bm *models.BuiltinMetric) error {
	// For non-English metrics, we don't merge by expression
	// In current implementation, user must have a zh_CN version of the metric
	// so we can use zh_CN as the key
	if existingId, ok := b.expressionIdMap[bm.Expression]; ok {
		// Update the existing metric with the new one
		if existingMetric, exists := b.bm[existingId]; exists {
			// We only need zh_CN as the key
			existingMetric.Translation = mergeTranslations(existingMetric.Translation, bm.Translation)
			// Update the existing metric with the new one
			b.bm[existingId] = existingMetric
		}
	} else {
		b.bm[bm.ID] = bm
		b.expressionIdMap[bm.Expression] = bm.ID
	}
	b.statTotal++
	b.statLastUpdated = time.Now().Unix()
	return nil
}

func mergeTranslations(existingTranslations, newTranslations []models.Translation) []models.Translation {
	translationMap := make(map[string]models.Translation)

	// Add existing translations to the map
	for _, t := range existingTranslations {
		translationMap[t.Lang] = t
	}

	// Add new translations to the map, overwriting existing ones if necessary
	for _, t := range newTranslations {
		translationMap[t.Lang] = t
	}

	// Convert the map back to a slice
	var mergedTranslations []models.Translation
	for _, t := range translationMap {
		mergedTranslations = append(mergedTranslations, t)
	}

	return mergedTranslations
}

func (b *BuiltinMetricCacheType) BuiltinMetricGets(lang, collector, typ, query, unit string, limit, offset int) ([]*models.BuiltinMetric, int, error) {
	logger.Debug("BuiltinMetricGets",
		"lang", lang,
		"collector", collector,
		"typ", typ,
		"query", query,
		"unit", unit,
		"limit", limit,
		"offset", offset,
	)
	var filteredMetrics []*models.BuiltinMetric

	for _, metric := range b.bm {
		if !applyFilter(metric, collector, typ, query, unit) {
			logger.Debug("BuiltinMetricGets",
				"lang", lang,
				"collector", collector,
				"typ", typ,
				"query", query,
				"unit", unit,
				"metric", metric,
				"filtered", false,
			)
			continue
		}

		// Apply language
		trans, err := getTranslationWithLanguage(metric, lang)
		if err != nil {
			continue // Skip if translation not found
		}
		metric.Name = trans.Name
		metric.Note = trans.Note

		filteredMetrics = append(filteredMetrics, metric)
	}

	sort.Slice(filteredMetrics, func(i, j int) bool {
		if filteredMetrics[i].Collector != filteredMetrics[j].Collector {
			return filteredMetrics[i].Collector < filteredMetrics[j].Collector
		}
		if filteredMetrics[i].Typ != filteredMetrics[j].Typ {
			return filteredMetrics[i].Typ < filteredMetrics[j].Typ
		}
		return filteredMetrics[i].Name < filteredMetrics[j].Name
	})

	if offset > len(filteredMetrics) {
		return nil, 0, nil
	}

	end := offset + limit
	if end > len(filteredMetrics) {
		end = len(filteredMetrics)
	}

	return filteredMetrics[offset:end], len(filteredMetrics), nil
}

func getTranslationWithLanguage(bm *models.BuiltinMetric, lang string) (*models.Translation, error) {
	var defaultTranslation *models.Translation
	for _, t := range bm.Translation {
		if t.Lang == lang {
			return &t, nil
		}

		if t.Lang == "en_US" {
			defaultTranslation = &t
		}
	}

	if defaultTranslation != nil {
		return defaultTranslation, nil
	}

	return nil, errors.New("translation not found")
}

func applyFilter(metric *models.BuiltinMetric, collector, typ, query, unit string) bool {
	return (metric.Collector == collector || collector == "") &&
		(metric.Typ == typ || typ == "") &&
		(containsUnit(unit, metric.Unit) || unit == "") &&
		(applyQueryFilter(metric, query) || query == "")
}

func containsUnit(unit, metricUnit string) bool {
	us := strings.Split(unit, ",")
	for _, u := range us {
		if u == metricUnit {
			return true
		}
	}
	return false
}

func applyQueryFilter(metric *models.BuiltinMetric, query string) bool {
	qs := strings.Split(query, " ")
	for _, q := range qs {
		if strings.HasPrefix(q, "-") {
			q = strings.TrimPrefix(q, "-")
			if strings.Contains(metric.Name, q) || strings.Contains(metric.Note, q) || strings.Contains(metric.Expression, q) {
				return false
			}
		} else {
			if !strings.Contains(metric.Name, q) && !strings.Contains(metric.Note, q) && !strings.Contains(metric.Expression, q) {
				return false
			}
		}
	}
	return true
}

func (b *BuiltinMetricCacheType) BuiltinMetricTypes(lang, collector, query string) []string {
	typeSet := set.NewStringSet()

	for _, metric := range b.bm {
		if !applyFilter(metric, collector, "", query, "") {
			continue
		}

		typeSet.Add(metric.Typ)
	}

	return typeSet.ToSlice()
}

func (b *BuiltinMetricCacheType) BuiltinMetricCollectors(lang, typ, query string) []string {
	collectorSet := set.NewStringSet()

	for _, metric := range b.bm {
		if !applyFilter(metric, "", typ, query, "") {
			continue
		}

		collectorSet.Add(metric.Collector)
	}

	return collectorSet.ToSlice()
}
