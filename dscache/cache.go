package dscache

import (
	"sync"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/toolkits/pkg/logger"
)

type Cache struct {
	datas map[string]map[int64]datasource.Datasource
	mutex *sync.RWMutex
}

var DsCache = Cache{
	datas: make(map[string]map[int64]datasource.Datasource),
	mutex: new(sync.RWMutex),
}

func (cs *Cache) Put(cate string, dsId int64, ds datasource.Datasource) {
	cs.mutex.Lock()
	if _, found := cs.datas[cate]; !found {
		cs.datas[cate] = make(map[int64]datasource.Datasource)
	}

	if _, found := cs.datas[cate][dsId]; found {
		if cs.datas[cate][dsId].Equal(ds) {
			cs.mutex.Unlock()
			return
		}
	}
	cs.mutex.Unlock()

	// InitClient() 在用户配置错误或远端不可用时, 会非常耗时, mutex被长期持有, 导致Get()会超时
	err := ds.InitClient()
	if err != nil {
		logger.Errorf("init plugin:%s %d %+v client fail: %v", cate, dsId, ds, err)
		return
	}

	logger.Debugf("init plugin:%s %d %+v client success", cate, dsId, ds)
	cs.mutex.Lock()
	cs.datas[cate][dsId] = ds
	cs.mutex.Unlock()
}

func (cs *Cache) Get(cate string, dsId int64) (datasource.Datasource, bool) {
	cs.mutex.RLock()
	defer cs.mutex.RUnlock()
	if _, found := cs.datas[cate]; !found {
		return nil, false
	}

	if _, found := cs.datas[cate][dsId]; !found {
		return nil, false
	}

	return cs.datas[cate][dsId], true
}
