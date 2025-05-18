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
	builtinMetricsByFile map[string]*models.BuiltinMetric // key: expression
	builtinMetricsByDB   map[string]*models.BuiltinMetric // key: expression
}

func NewBuiltinMetricCache(ctx *ctx.Context, stats *Stats, builtinIntegrationsDir string) *BuiltinMetricCacheType {
	bm := &BuiltinMetricCacheType{
		statTotal:              -1,
		statLastUpdated:        -1,
		ctx:                    ctx,
		stats:                  stats,
		builtinIntegrationsDir: builtinIntegrationsDir,
		builtinMetricsByFile:   make(map[string]*models.BuiltinMetric),
		builtinMetricsByDB:     make(map[string]*models.BuiltinMetric),
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
	b.Lock()
	defer b.Unlock()

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

					// Metrics in file contain latest structure, so we can directly add them
					b.builtinMetricsByFile[metric.Expression] = &metric
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
		if err := b.syncBuiltinMetricsByDB(); err != nil {
			logger.Warning("failed to sync datasources:", err)
		}
	}
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

func (b *BuiltinMetricCacheType) Set(builtinMetricsByDB []*models.BuiltinMetric, total, lastUpdated int64) {
	b.Lock()
	defer b.Unlock()

	builtinMetricsByDBList := make(map[string][]*models.BuiltinMetric)

	// Clear the old cache from DB
	b.builtinMetricsByDB = make(map[string]*models.BuiltinMetric)

	for _, metric := range builtinMetricsByDB {
		b.appendBuiltinMetric(metric, builtinMetricsByDBList)
	}

	// Convert to builtinMetricsByDB
	b.convertBuiltinMetricByDB(builtinMetricsByDBList)

	// only one goroutine used, so no need lock
	b.statTotal = total
	b.statLastUpdated = lastUpdated
}

func (b *BuiltinMetricCacheType) convertBuiltinMetricByDB(builtinMetricsCacheList map[string][]*models.BuiltinMetric) {
	for expression, builtinMetrics := range builtinMetricsCacheList {
		// Sort by id and get the first one
		sort.Slice(builtinMetrics, func(i, j int) bool {
			return builtinMetrics[i].ID < builtinMetrics[j].ID
		})

		currentBuiltinMetric := builtinMetrics[0]
		// User have no customed translation, so we can merge it
		if len(currentBuiltinMetric.Translation) == 0 {
			for _, bm := range builtinMetrics {
				currentBuiltinMetric.Translation = mergeTranslations(
					getDefaultTranslation(currentBuiltinMetric),
					getDefaultTranslation(bm),
				)
			}
		}

		b.builtinMetricsByDB[expression] = currentBuiltinMetric
	}
}

func getDefaultTranslation(bm *models.BuiltinMetric) []models.Translation {
	if len(bm.Translation) != 0 {
		return bm.Translation
	}

	return []models.Translation{{
		Lang: bm.Lang,
		Name: bm.Name,
		Note: bm.Note,
	}}
}

// Add new builtin metric, ensuring cache consistency for duplicate expressions
func (b *BuiltinMetricCacheType) appendBuiltinMetric(
	bm *models.BuiltinMetric,
	builtinMetricsCacheList map[string][]*models.BuiltinMetric,
) {
	builtinMetrics, ok := builtinMetricsCacheList[bm.Expression]
	if !ok {
		builtinMetrics = []*models.BuiltinMetric{}
	}

	builtinMetrics = append(builtinMetrics, bm)
	builtinMetricsCacheList[bm.Expression] = builtinMetrics
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
	sources := []map[string]*models.BuiltinMetric{
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
	return (collector == "" || metric.Collector == collector) &&
		(typ == "" || metric.Typ == typ) &&
		(unit == "" || containsUnit(unit, metric.Unit)) &&
		(query == "" || applyQueryFilter(metric, query))
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

	sources := []map[string]*models.BuiltinMetric{
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

	sources := []map[string]*models.BuiltinMetric{
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
