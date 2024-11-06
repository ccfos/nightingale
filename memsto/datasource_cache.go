package memsto

import (
	"log"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/gin-gonic/gin"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type DatasourceCacheType struct {
	statTotal           int64
	statLastUpdated     int64
	ctx                 *ctx.Context
	stats               *Stats
	DatasourceCheckHook func(*gin.Context) bool
	DatasourceFilter    func([]*models.Datasource, *models.User) []*models.Datasource

	sync.RWMutex
	ds         map[int64]*models.Datasource // key: id
	dsNameToID map[string]int64
}

func NewDatasourceCache(ctx *ctx.Context, stats *Stats) *DatasourceCacheType {
	ds := &DatasourceCacheType{
		statTotal:           -1,
		statLastUpdated:     -1,
		ctx:                 ctx,
		stats:               stats,
		ds:                  make(map[int64]*models.Datasource),
		dsNameToID:          make(map[string]int64),
		DatasourceCheckHook: func(ctx *gin.Context) bool { return false },
		DatasourceFilter:    func(ds []*models.Datasource, user *models.User) []*models.Datasource { return ds },
	}
	ds.SyncDatasources()
	return ds
}

func (d *DatasourceCacheType) GetIDsByDsQueries(cate string, datasourceQueries []models.DatasourceQuery) []int64 {
	d.Lock()
	defer d.Unlock()
	// 先确定 DatasourceQueries 所对应的数据源范围
	idMap := make(map[int64]struct{})
	NameMap := make(map[string]int64)
	for id, ds := range d.ds {
		if ds.PluginType != cate {
			continue
		}
		idMap[id] = struct{}{}
		NameMap[ds.Name] = id
	}
	return models.GetDatasourceIDsByDatasourceQueries(datasourceQueries, idMap, NameMap)
}

func (d *DatasourceCacheType) StatChanged(total, lastUpdated int64) bool {
	if d.statTotal == total && d.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (d *DatasourceCacheType) Set(ds map[int64]*models.Datasource, dsNameToID map[string]int64, total, lastUpdated int64) {
	d.Lock()
	d.ds = ds
	d.dsNameToID = dsNameToID
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
		dumper.PutSyncRecord("datasources", start.Unix(), -1, -1, "failed to query statistics: "+err.Error())
		return errors.WithMessage(err, "failed to call DatasourceStatistics")
	}

	if !d.StatChanged(stat.Total, stat.LastUpdated) {
		d.stats.GaugeCronDuration.WithLabelValues("sync_datasources").Set(0)
		d.stats.GaugeSyncNumber.WithLabelValues("sync_datasources").Set(0)
		dumper.PutSyncRecord("datasources", start.Unix(), -1, -1, "not changed")
		return nil
	}

	ds, dsNameToID, err := models.DatasourceGetMap(d.ctx)
	if err != nil {
		dumper.PutSyncRecord("datasources", start.Unix(), -1, -1, "failed to query records: "+err.Error())
		return errors.WithMessage(err, "failed to call DatasourceGetMap")
	}

	d.Set(ds, dsNameToID, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	d.stats.GaugeCronDuration.WithLabelValues("sync_datasources").Set(float64(ms))
	d.stats.GaugeSyncNumber.WithLabelValues("sync_datasources").Set(float64(len(ds)))

	logger.Infof("timer: sync datasources done, cost: %dms, number: %d", ms, len(ds))
	dumper.PutSyncRecord("datasources", start.Unix(), ms, len(ds), "success")

	return nil
}
