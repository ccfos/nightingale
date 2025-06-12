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
	builtinMetricsByFile  map[int64]*models.BuiltinMetric // key: uuid
	expressionIdMapByFile map[string]int64                // key: expression, value: uuid
	builtinMetricsByDB    map[int64]*models.BuiltinMetric // key: uuid
	expressionIdMapByDB   map[string]int64                // key: expression, value: uuid
}

func NewBuiltinMetricCache(ctx *ctx.Context, stats *Stats, builtinIntegrationsDir string) *BuiltinMetricCacheType {
	bm := &BuiltinMetricCacheType{
		statTotal:              -1,
		statLastUpdated:        -1,
		ctx:                    ctx,
		stats:                  stats,
		builtinIntegrationsDir: builtinIntegrationsDir,
		builtinMetricsByFile:   make(map[int64]*models.BuiltinMetric),
		expressionIdMapByFile:  make(map[string]int64),
		builtinMetricsByDB:     make(map[int64]*models.BuiltinMetric),
		expressionIdMapByDB:    make(map[string]int64),
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
	b.initBuiltinMetricByFile()

	err := b.syncBuiltinMetricsByDB()
	if err != nil {
		logger.Errorf("failed to sync builtin components: %v", err)
	}

	go b.loopSyncBuiltinMetricsByDB()
}

func (b *BuiltinMetricCacheType) initBuiltinMetricByFile() error {
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

					logger.Infof("add builtin metric %s", metric.Name)
					b.addBuiltinMetricByFile(&metric)
					logger.Infof("len  of builtin metrics: %d", len(b.builtinMetricsByFile))
				}
			}
		} else if err != nil {
			logger.Warningf("read builtin component metrics dir fail %s %v", component.Ident, err)
		}
	}

	return nil
}

func (b *BuiltinMetricCacheType) loopSyncBuiltinMetricsByDB() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		// Current metric need to be cleaned up before sync
		// to avoid duplicate metrics.
		b.CleanupBuiltinMetricsByDB()
		if err := b.syncBuiltinMetricsByDB(); err != nil {
			logger.Warning("failed to sync datasources:", err)
		}
	}
}

func (b *BuiltinMetricCacheType) CleanupBuiltinMetricsByDB() {
	b.Lock()
	defer b.Unlock()

	// Clear the cache
	b.builtinMetricsByDB = make(map[int64]*models.BuiltinMetric)
	b.expressionIdMapByDB = make(map[string]int64)
}

func (b *BuiltinMetricCacheType) syncBuiltinMetricsByDB() error {
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
		b.addBuiltinMetricByDB(metric)
	}

	// only one goroutine used, so no need lock
	b.statTotal = total
	b.statLastUpdated = lastUpdated
}

func (b *BuiltinMetricCacheType) GetByBuiltinMetricId(id int64) (*models.BuiltinMetric, error) {
	b.RLock()
	defer b.RLock()

	source := []map[int64]*models.BuiltinMetric{
		b.builtinMetricsByFile,
		b.builtinMetricsByDB,
	}
	for _, metrics := range source {
		if bp, ok := metrics[id]; ok {
			return bp, nil
		}
	}

	return nil, errors.New("builtin metric not found")
}

func (b *BuiltinMetricCacheType) addBuiltinMetricByFile(bm *models.BuiltinMetric) error {
	b.Lock()
	defer b.Unlock()

	return b.addBuiltinMetric(bm, b.expressionIdMapByFile, b.builtinMetricsByFile)
}

func (b *BuiltinMetricCacheType) addBuiltinMetricByDB(bm *models.BuiltinMetric) error {
	b.Lock()
	defer b.Unlock()

	return b.addBuiltinMetric(bm, b.expressionIdMapByDB, b.builtinMetricsByDB)
}

// Add new builtin metric, ensuring cache consistency for duplicate expressions
func (b *BuiltinMetricCacheType) addBuiltinMetric(
	bm *models.BuiltinMetric,
	expressionIdMap map[string]int64,
	builtinMetrics map[int64]*models.BuiltinMetric,
) error {
	if _, exists := builtinMetrics[bm.UUID]; exists {
		return errors.Errorf("builtin component with UUID %d already exists", bm.UUID)
	}

	// Merge to existing metric with same expression
	if bm.Lang == "en_US" {
		return b.addEnglishMetric(bm, expressionIdMap, builtinMetrics)
	}

	return b.addNonEnglishMetric(bm, expressionIdMap, builtinMetrics)
}

func (b *BuiltinMetricCacheType) addEnglishMetric(
	bm *models.BuiltinMetric,
	expressionIdMap map[string]int64,
	builtinMetrics map[int64]*models.BuiltinMetric,
) error {
	if existingId, ok := expressionIdMap[bm.Expression]; ok {
		// Update the existing metric with the new one
		if existingMetric, exists := builtinMetrics[existingId]; exists {
			// Merge translation to current metric
			bm.Translation = mergeTranslations(existingMetric.Translation, bm.Translation)
		}
		// Delete the old metric
		delete(builtinMetrics, existingId)
	}
	// Direct update
	builtinMetrics[bm.UUID] = bm
	expressionIdMap[bm.Expression] = bm.UUID
	b.statTotal++
	b.statLastUpdated = time.Now().Unix()
	return nil
}

func (b *BuiltinMetricCacheType) addNonEnglishMetric(
	bm *models.BuiltinMetric,
	expressionIdMap map[string]int64,
	builtinMetrics map[int64]*models.BuiltinMetric,
) error {
	// For non-English metrics, we don't merge by expression
	// In current implementation, user must have a zh_CN version of the metric
	// so we can use zh_CN as the key
	if existingId, ok := expressionIdMap[bm.Expression]; ok {
		// Update the existing metric with the new one
		if existingMetric, exists := builtinMetrics[existingId]; exists {
			// We only need zh_CN as the key
			existingMetric.Translation = mergeTranslations(existingMetric.Translation, bm.Translation)
			// Update the existing metric with the new one
			builtinMetrics[existingId] = existingMetric
		}
	} else {
		builtinMetrics[bm.UUID] = bm
		expressionIdMap[bm.Expression] = bm.UUID
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
	var filteredMetrics []*models.BuiltinMetric
	sources := []map[int64]*models.BuiltinMetric{
		b.builtinMetricsByFile,
		b.builtinMetricsByDB,
	}

	// Get all metrics from both file and DB caches with filtering applied
	for _, metrics := range sources {
		for _, metric := range metrics {
			if !applyFilter(metric, collector, typ, query, unit) {
				continue
			}

			// Apply language
			trans, err := getTranslationWithLanguage(metric, lang)
			if err != nil {
				logger.Errorf("Error getting translation for metric %s: %v", metric.Name, err)
				continue // Skip if translation not found
			}
			metric.Name = trans.Name
			metric.Note = trans.Note

			filteredMetrics = append(filteredMetrics, metric)
		}
	}

	// Sort metrics
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

	// Apply pagination
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

	return nil, errors.Errorf("translation not found for metric %s", bm.Name)
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

	sources := []map[int64]*models.BuiltinMetric{
		b.builtinMetricsByFile,
		b.builtinMetricsByDB,
	}

	for _, metrics := range sources {
		for _, metric := range metrics {
			if !applyFilter(metric, collector, "", query, "") {
				continue
			}

			typeSet.Add(metric.Typ)
		}
	}

	return typeSet.ToSlice()
}

func (b *BuiltinMetricCacheType) BuiltinMetricCollectors(lang, typ, query string) []string {
	collectorSet := set.NewStringSet()

	sources := []map[int64]*models.BuiltinMetric{
		b.builtinMetricsByFile,
		b.builtinMetricsByDB,
	}

	for _, metrics := range sources {
		for _, metric := range metrics {
			if !applyFilter(metric, "", typ, query, "") {
				continue
			}

			collectorSet.Add(metric.Collector)
		}
	}

	return collectorSet.ToSlice()
}
