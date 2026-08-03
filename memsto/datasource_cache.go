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

	// syncMu 串行化 syncDatasources 的「读 stat → 判断 → 读库 → Set」全流程。
	// SyncOnce 让 upsert 的 HTTP handler 也能触发同步，会与 9 秒一次的循环并发，
	// 不整段互斥的话慢的那次可能用旧快照覆盖新快照。
	// 必须与下面的 RWMutex 分开：那把锁保护 ds / CateToIDs 的读取（GetById 等），
	// 拿它包住 DB 查询会把所有读者一起堵死。
	syncMu sync.Mutex

	sync.RWMutex
	ds          map[int64]*models.Datasource            // key: id value: datasource
	CateToIDs   map[string]map[int64]*models.Datasource // key1: cate key2: id value: datasource
	CateToNames map[string]map[string]int64             // key1: cate key2: name value: id
}

func NewDatasourceCache(ctx *ctx.Context, stats *Stats) *DatasourceCacheType {
	ds := &DatasourceCacheType{
		statTotal:           -1,
		statLastUpdated:     -1,
		ctx:                 ctx,
		stats:               stats,
		ds:                  make(map[int64]*models.Datasource),
		CateToIDs:           make(map[string]map[int64]*models.Datasource),
		CateToNames:         make(map[string]map[string]int64),
		DatasourceCheckHook: func(ctx *gin.Context) bool { return false },
		DatasourceFilter:    func(ds []*models.Datasource, user *models.User) []*models.Datasource { return ds },
	}
	ds.SyncDatasources()
	return ds
}

func (d *DatasourceCacheType) GetIDsByDsCateAndQueries(cate string, datasourceQueries []models.DatasourceQuery) []int64 {
	d.Lock()
	defer d.Unlock()
	return models.GetDatasourceIDsByDatasourceQueries(datasourceQueries, d.CateToIDs[cate], d.CateToNames[cate])
}

func (d *DatasourceCacheType) StatChanged(total, lastUpdated int64) bool {
	d.RLock()
	defer d.RUnlock()

	if d.statTotal == total && d.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (d *DatasourceCacheType) Set(ds map[int64]*models.Datasource, total, lastUpdated int64) {
	cateToDs := make(map[string]map[int64]*models.Datasource)
	cateToNames := make(map[string]map[string]int64)
	for _, datasource := range ds {
		if _, exists := cateToDs[datasource.PluginType]; !exists {
			cateToDs[datasource.PluginType] = make(map[int64]*models.Datasource)
		}
		cateToDs[datasource.PluginType][datasource.Id] = datasource
		if _, exists := cateToNames[datasource.PluginType]; !exists {
			cateToNames[datasource.PluginType] = make(map[string]int64)
		}
		cateToNames[datasource.PluginType][datasource.Name] = datasource.Id
	}
	d.Lock()
	d.CateToIDs = cateToDs
	d.ds = ds
	d.CateToNames = cateToNames
	d.statTotal = total
	d.statLastUpdated = lastUpdated
	d.Unlock()
}

func (d *DatasourceCacheType) GetById(id int64) *models.Datasource {
	d.RLock()
	defer d.RUnlock()
	return d.ds[id]
}

// GetByCate 返回某个 plugin_type 下的全部数据源快照。返回的是 map 迭代顺序，
// 调用方若需要稳定输出（如写进 API 响应）要自行排序。
func (d *DatasourceCacheType) GetByCate(cate string) []*models.Datasource {
	d.RLock()
	defer d.RUnlock()

	lst := make([]*models.Datasource, 0, len(d.CateToIDs[cate]))
	for _, ds := range d.CateToIDs[cate] {
		lst = append(lst, ds)
	}
	return lst
}

// GetStat 返回缓存当前同步到的 (total, lastUpdated)。外部据此判断 GetById 读到的快照版本，
// 与缓存内部 StatChanged 判定同源——缓存何时刷新内容，该值就何时变化。
func (d *DatasourceCacheType) GetStat() (total, lastUpdated int64) {
	d.RLock()
	defer d.RUnlock()
	return d.statTotal, d.statLastUpdated
}

func (d *DatasourceCacheType) SyncDatasources() {
	err := d.syncDatasources()
	if err != nil {
		log.Fatalln("failed to sync datasources:", err)
	}

	go d.loopSyncDatasources()
}

// SyncOnce 立即同步一次数据源缓存，不启动循环、失败也不终止进程。
// 供配置写入后（如 upsert）主动调用，让新数据源立刻对 proxy / 查询可见，
// 而不必等待 9 秒的下一个同步周期 —— 否则保存后立即查询会报 "no such datasource"。
func (d *DatasourceCacheType) SyncOnce() error {
	return d.syncDatasources()
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
	// 与 loopSyncDatasources 及各个 SyncOnce 调用方串行执行，详见 syncMu 的注释
	d.syncMu.Lock()
	defer d.syncMu.Unlock()

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

	ds, err := models.DatasourceGetMap(d.ctx)
	if err != nil {
		dumper.PutSyncRecord("datasources", start.Unix(), -1, -1, "failed to query records: "+err.Error())
		return errors.WithMessage(err, "failed to call DatasourceGetMap")
	}

	d.Set(ds, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	d.stats.GaugeCronDuration.WithLabelValues("sync_datasources").Set(float64(ms))
	d.stats.GaugeSyncNumber.WithLabelValues("sync_datasources").Set(float64(len(ds)))
	dumper.PutSyncRecord("datasources", start.Unix(), ms, len(ds), "success")

	return nil
}
