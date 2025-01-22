package dscache

import (
	"fmt"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
)

type Cache struct {
	datas            map[string]map[int64]datasource.Datasource
	datasourceStatus map[string]string
	mutex            *sync.RWMutex
}

var DsCache = Cache{
	datas:            make(map[string]map[int64]datasource.Datasource),
	datasourceStatus: make(map[string]string),
	mutex:            new(sync.RWMutex),
}

func (cs *Cache) Put(cate string, dsId int64, name string, ds datasource.Datasource) {
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
		cs.SetStatus(name, fmt.Sprintf("%s init plugin:%s %d %+v client fail: %v", time.Now().Format("2006-01-02 15:04:05"), cate, dsId, ds, err))
		return
	}

	cs.SetStatus(name, fmt.Sprintf("%s init plugin:%s %d %+v client success", time.Now().Format("2006-01-02 15:04:05"), cate, dsId, ds))
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

func (cs *Cache) GetAllStatus() map[string]string {
	cs.mutex.RLock()
	defer cs.mutex.RUnlock()
	return cs.datasourceStatus
}

func (cs *Cache) SetStatus(cate string, status string) {
	cs.mutex.Lock()
	cs.datasourceStatus[cate] = status
	cs.mutex.Unlock()
}
