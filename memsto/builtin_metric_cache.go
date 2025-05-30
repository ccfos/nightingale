package memsto

import (
	"encoding/json"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
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
	bc map[int64]*models.BuiltinMetric // key: id
}

func NewBuiltinMetricCacheType(ctx *ctx.Context, stats *Stats, builtinIntegrationsDir string) *BuiltinMetricCacheType {
	bc := &BuiltinMetricCacheType{
		statTotal:              -1,
		statLastUpdated:        -1,
		ctx:                    ctx,
		stats:                  stats,
		builtinIntegrationsDir: builtinIntegrationsDir,
		bc:                     make(map[int64]*models.BuiltinMetric),
	}

	bc.SyncBuiltinMetrics()
	return bc
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

		// get logo name
		// /api/n9e/integrations/icon/AliYun/aliyun.png
		files, err := file.FilesUnder(componentDir + "/icon")
		if err == nil && len(files) > 0 {
			component.Logo = "/api/n9e/integrations/icon/" + component.Ident + "/" + files[0]
		} else if err != nil {
			logger.Warningf("read builtin component icon dir fail %s %v", component.Ident, err)
		}

		// get description
		files, err = file.FilesUnder(componentDir + "/markdown")
		if err == nil && len(files) > 0 {
			var readmeFile string
			for _, file := range files {
				if strings.HasSuffix(strings.ToLower(file), "md") {
					readmeFile = componentDir + "/markdown/" + file
					break
				}
			}
			if readmeFile != "" {
				component.Readme, _ = file.ReadString(readmeFile)
			}
		} else if err != nil {
			logger.Warningf("read builtin component markdown dir fail %s %v", component.Ident, err)
		}

		// metrics
		files, err = file.FilesUnder(componentDir + "/metrics")
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

					b.bc[metric.ID] = &metric
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
		if err := b.syncBuiltinMetrics(); err != nil {
			logger.Warning("failed to sync datasources:", err)
		}
	}
}

func (b *BuiltinMetricCacheType) syncBuiltinMetrics() error {
	start := time.Now()

	stat, err := models.BuiltinMetricStatistics(b.ctx)
	if err != nil {
		dumper.PutSyncRecord("builtin_components", start.Unix(), -1, -1, "failed to query statistics: "+err.Error())
		return errors.WithMessage(err, "failed to exec BuiltinMetricStatistics")
	}

	if !b.StatChanged(stat.Total, stat.LastUpdated) {
		b.stats.GaugeCronDuration.WithLabelValues("sync_builtin_components").Set(0)
		b.stats.GaugeSyncNumber.WithLabelValues("sync_builtin_components").Set(0)
		dumper.PutSyncRecord("builtin_components", start.Unix(), -1, -1, "not changed")
		return nil
	}

	bc, err := models.BuiltinMetricGetAllMap(b.ctx)
	if err != nil {
		dumper.PutSyncRecord("builtin_components", start.Unix(), -1, -1, "failed to query records: "+err.Error())
		return errors.WithMessage(err, "failed to call BuiltinMetricGetMap")
	}

	b.Set(bc, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	b.stats.GaugeCronDuration.WithLabelValues("sync_builtin_components").Set(float64(ms))
	b.stats.GaugeSyncNumber.WithLabelValues("sync_builtin_components").Set(float64(len(bc)))

	logger.Infof("timer: sync builtin components done, cost: %dms, number: %d", ms, len(bc))
	dumper.PutSyncRecord("builtin_components", start.Unix(), ms, len(bc), "success")

	return nil
}

func (b *BuiltinMetricCacheType) Set(bc map[int64]*models.BuiltinMetric, total, lastUpdated int64) {
	b.Lock()
	b.bc = bc
	b.Unlock()

	// only one goroutine used, so no need lock
	b.statTotal = total
	b.statLastUpdated = lastUpdated
}

func (b *BuiltinMetricCacheType) GetByBuiltinMetricId(id int64) *models.BuiltinMetric {
	b.RLock()
	defer b.RLock()
	return b.bc[id]
}

func (b *BuiltinMetricCacheType) addBuiltinMetric(bc *models.BuiltinMetric) error {
	b.Lock()
	defer b.Unlock()

	if _, exists := b.bc[bc.ID]; exists {
		return errors.New("builtin component already exists")
	}

	b.bc[bc.ID] = bc
	b.statTotal++
	b.statLastUpdated = time.Now().Unix()

	return nil
}
