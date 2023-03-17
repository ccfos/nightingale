package memsto

import (
	"log"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type DatasourceCacheType struct {
	statTotal       int64
	statLastUpdated int64
	ctx             *ctx.Context
	stats           *Stats

	sync.RWMutex
	ds map[int64]*models.Datasource // key: id
}

func NewDatasourceCache(ctx *ctx.Context, stats *Stats) *DatasourceCacheType {
	ds := &DatasourceCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		stats:           stats,
		ds:              make(map[int64]*models.Datasource),
	}
	ds.SyncDatasources()
	return ds
}

func (d *DatasourceCacheType) StatChanged(total, lastUpdated int64) bool {
	if d.statTotal == total && d.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (d *DatasourceCacheType) Set(ds map[int64]*models.Datasource, total, lastUpdated int64) {
	d.Lock()
	d.ds = ds
	d.Unlock()

	// only one goroutine used, so no need lock
	d.statTotal = total
	d.statLastUpdated = lastUpdated
}

func (d *DatasourceCacheType) GetById(id int64) *models.Datasource {
	d.RLock()
	defer d.RUnlock()
	return d.ds[id]
}

func (d *DatasourceCacheType) SyncDatasources() {
	err := d.syncDatasources()
	if err != nil {
		log.Fatalln("failed to sync datasources:", err)
	}

	go d.loopSyncDatasources()
}

func (d *DatasourceCacheType) loopSyncDatasources() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := d.syncDatasources(); err != nil {
			logger.Warning("failed to sync datasources:", err)
		}
	}
}

func (d *DatasourceCacheType) syncDatasources() error {
	start := time.Now()

	stat, err := models.DatasourceStatistics(d.ctx)
	if err != nil {
		return errors.WithMessage(err, "failed to call DatasourceStatistics")
	}

	if !d.StatChanged(stat.Total, stat.LastUpdated) {
		d.stats.GaugeCronDuration.WithLabelValues("sync_datasources").Set(0)
		d.stats.GaugeSyncNumber.WithLabelValues("sync_datasources").Set(0)

		logger.Debug("datasource not changed")
		return nil
	}

	m, err := models.DatasourceGetMap(d.ctx)
	if err != nil {
		return errors.WithMessage(err, "failed to call DatasourceGetMap")
	}

	d.Set(m, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	d.stats.GaugeCronDuration.WithLabelValues("sync_datasources").Set(float64(ms))
	d.stats.GaugeSyncNumber.WithLabelValues("sync_datasources").Set(float64(len(m)))

	logger.Infof("timer: sync datasources done, cost: %dms, number: %d", ms, len(m))

	return nil
}
