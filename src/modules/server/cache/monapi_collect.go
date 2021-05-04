package cache

import (
	"sync"

	"github.com/didi/nightingale/v4/src/common/dataobj"
	"github.com/didi/nightingale/v4/src/models"
)

type CollectCacheMap struct {
	sync.RWMutex
	Data map[string]*models.Collect
}

var CollectCache *CollectCacheMap

func NewCollectCache() *CollectCacheMap {
	return &CollectCacheMap{Data: make(map[string]*models.Collect)}
}

func (c *CollectCacheMap) GetBy(endpoint string) *models.Collect {
	c.RLock()
	defer c.RUnlock()

	return c.Data[endpoint]
}

func (c *CollectCacheMap) Set(endpoint string, collect *models.Collect) {
	c.Lock()
	defer c.Unlock()

	c.Data[endpoint] = collect
	return
}

func (c *CollectCacheMap) SetAll(strasMap map[string]*models.Collect) {
	c.Lock()
	defer c.Unlock()

	c.Data = strasMap
	return
}

type ApiCollectCacheMap struct {
	sync.RWMutex
	Data map[string][]*models.ApiCollect
}

var ApiCollectCache *ApiCollectCacheMap

func NewApiCollectCache() *ApiCollectCacheMap {
	return &ApiCollectCacheMap{Data: make(map[string][]*models.ApiCollect)}
}

func (c *ApiCollectCacheMap) GetBy(node string) []*models.ApiCollect {
	c.RLock()
	defer c.RUnlock()

	return c.Data[node]
}

func (c *ApiCollectCacheMap) Set(node string, collects []*models.ApiCollect) {
	c.Lock()
	defer c.Unlock()

	c.Data[node] = collects
	return
}

func (c *ApiCollectCacheMap) SetAll(data map[string][]*models.ApiCollect) {
	c.Lock()
	defer c.Unlock()

	c.Data = data
	return
}

//snmp
type SnmpCollectCacheMap struct {
	sync.RWMutex
	Data map[string][]*dataobj.IPAndSnmp
}

var SnmpCollectCache *SnmpCollectCacheMap
var SnmpHWCache *SnmpHWCacheMap

func NewSnmpCollectCache() *SnmpCollectCacheMap {
	return &SnmpCollectCacheMap{Data: make(map[string][]*dataobj.IPAndSnmp)}
}

func (c *SnmpCollectCacheMap) GetBy(node string) []*dataobj.IPAndSnmp {
	c.RLock()
	defer c.RUnlock()

	return c.Data[node]
}

func (c *SnmpCollectCacheMap) Set(node string, collects []*dataobj.IPAndSnmp) {
	c.Lock()
	defer c.Unlock()

	c.Data[node] = collects
	return
}

func (c *SnmpCollectCacheMap) SetAll(data map[string][]*dataobj.IPAndSnmp) {
	c.Lock()
	defer c.Unlock()

	c.Data = data
	return
}

type SnmpHWCacheMap struct {
	sync.RWMutex
	Data map[string][]*models.NetworkHardware
}

func NewSnmpHWCache() *SnmpHWCacheMap {
	return &SnmpHWCacheMap{Data: make(map[string][]*models.NetworkHardware)}
}

func (c *SnmpHWCacheMap) GetBy(node string) []*models.NetworkHardware {
	c.RLock()
	defer c.RUnlock()

	return c.Data[node]
}

func (c *SnmpHWCacheMap) Set(node string, hws []*models.NetworkHardware) {
	c.Lock()
	defer c.Unlock()

	c.Data[node] = hws
	return
}

func (c *SnmpHWCacheMap) SetAll(data map[string][]*models.NetworkHardware) {
	c.Lock()
	defer c.Unlock()

	c.Data = data
	return
}
