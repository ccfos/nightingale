package memsto

import (
	"encoding/json"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/gin-gonic/gin"

	"github.com/pkg/errors"
	"github.com/tidwall/match"
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

func (d *DatasourceCacheType) GetIDsByDsQueries(datasourceQueriesJson []interface{}) []int64 {
	dsIDs := make(map[int64]struct{})
	for i := range datasourceQueriesJson {
		var q models.DatasourceQuery
		bytes, err := json.Marshal(datasourceQueriesJson[i])
		if err != nil {
			continue
		}

		if err = json.Unmarshal(bytes, &q); err != nil {
			continue
		}

		if q.MatchType == 0 {
			value := make([]int64, 0, len(q.Values))
			for v := range q.Values {
				val, err := strconv.Atoi(q.Values[v])
				if err != nil {
					continue
				}
				value = append(value, int64(val))
			}
			if q.Op == "in" {
				if len(value) == 1 && value[0] == models.DatasourceIdAll {
					for c := range d.ds {
						dsIDs[c] = struct{}{}
					}
					continue
				}

				for v := range value {
					dsIDs[value[v]] = struct{}{}
				}

			} else if q.Op == "not in" {
				for v := range value {
					delete(dsIDs, value[v])
				}
			}
		} else if q.MatchType == 1 {
			if q.Op == "in" {
				for dsName := range d.dsNameToID {
					for v := range q.Values {
						if match.Match(dsName, q.Values[v]) {
							dsIDs[d.dsNameToID[dsName]] = struct{}{}
						}
					}
				}
			} else if q.Op == "not in" {
				for dsName := range d.dsNameToID {
					for v := range q.Values {
						if match.Match(dsName, q.Values[v]) {
							dsIDs[d.dsNameToID[dsName]] = struct{}{}
						}
					}
				}
			}
		}
	}
	ids := make([]int64, 0, len(dsIDs))
	for c := range dsIDs {
		ids = append(ids, c)
	}
	return ids
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
