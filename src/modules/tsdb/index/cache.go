package index

import (
	"sync"

	"github.com/didi/nightingale/src/dataobj"
)

const (
	DefaultMaxCacheSize = 5000000 // 默认 最多500w个,太大了内存会耗尽
)

type DsTypeAndStep struct {
	DsType string `json:"dstype"`
	Step   int    `json:"step"`
}

// 索引缓存的元素数据结构
type IndexCacheItem struct {
	UUID interface{}
	Item *dataobj.TsdbItem
}

func NewIndexCacheItem(uuid interface{}, item *dataobj.TsdbItem) *IndexCacheItem {
	return &IndexCacheItem{UUID: uuid, Item: item}
}

// 索引缓存-基本缓存容器
type IndexCacheBase struct {
	sync.RWMutex
	maxSize int
	data    map[interface{}]*dataobj.TsdbItem
}

func NewIndexCacheBase(max int) *IndexCacheBase {
	return &IndexCacheBase{maxSize: max, data: make(map[interface{}]*dataobj.TsdbItem)}
}

func (i *IndexCacheBase) Put(key interface{}, item *dataobj.TsdbItem) {
	i.Lock()
	defer i.Unlock()
	i.data[key] = item
}

func (i *IndexCacheBase) Get(key interface{}) *dataobj.TsdbItem {
	i.RLock()
	defer i.RUnlock()
	return i.data[key]
}

func (i *IndexCacheBase) ContainsKey(key interface{}) bool {
	i.RLock()
	defer i.RUnlock()
	return i.data[key] != nil
}

func (i *IndexCacheBase) Size() int {
	i.RLock()
	defer i.RUnlock()
	return len(i.data)
}

func (i *IndexCacheBase) Keys() []interface{} {
	i.RLock()
	defer i.RUnlock()

	count := len(i.data)
	if count == 0 {
		return []interface{}{}
	}

	keys := make([]interface{}, 0, count)
	for key := range i.data {
		keys = append(keys, key)
	}

	return keys
}

func (i *IndexCacheBase) Remove(key interface{}) {
	i.Lock()
	defer i.Unlock()
	delete(i.data, key)
}
