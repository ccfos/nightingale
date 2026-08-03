package memsto

import (
	"sync"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

// SyncOnce 让 syncDatasources 可以被 upsert 的 HTTP handler 与 9 秒一次的循环并发调用，
// 于是 StatChanged 的读与 Set 的写落到了不同 goroutine 上。本用例在 -race 下跑，
// 用来钉住「statTotal / statLastUpdated 的读写必须同锁」这条约束。
func TestDatasourceCacheStatRace(t *testing.T) {
	d := &DatasourceCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ds:              make(map[int64]*models.Datasource),
		CateToIDs:       make(map[string]map[int64]*models.Datasource),
		CateToNames:     make(map[string]map[string]int64),
	}

	const rounds = 2000
	var wg sync.WaitGroup

	// 模拟并发的 syncDatasources 前半段：无锁读 stat 做判断
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < rounds; j++ {
				d.StatChanged(int64(j), int64(j))
			}
		}()
	}

	// 模拟 syncDatasources 后半段：持写锁刷新快照与 stat
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < rounds; j++ {
			d.Set(map[int64]*models.Datasource{
				1: {Id: 1, Name: "prom", PluginType: "prometheus"},
			}, int64(j), int64(j))
		}
	}()

	wg.Wait()
}
