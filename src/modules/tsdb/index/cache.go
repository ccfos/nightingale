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

func (this *IndexCacheBase) Put(key interface{}, item *dataobj.TsdbItem) {
	this.Lock()
	defer this.Unlock()
	this.data[key] = item
}

func (this *IndexCacheBase) Get(key interface{}) *dataobj.TsdbItem {
	this.RLock()
	defer this.RUnlock()
	return this.data[key]
}

func (this *IndexCacheBase) ContainsKey(key interface{}) bool {
	this.RLock()
	defer this.RUnlock()
	return this.data[key] != nil
}

func (this *IndexCacheBase) Size() int {
	this.RLock()
	defer this.RUnlock()
	return len(this.data)
}

func (this *IndexCacheBase) Keys() []interface{} {
	this.RLock()
	defer this.RUnlock()

	count := len(this.data)
	if count == 0 {
		return []interface{}{}
	}

	keys := make([]interface{}, 0, count)
	for key := range this.data {
		keys = append(keys, key)
	}

	return keys
}

func (this *IndexCacheBase) Remove(key interface{}) {
	this.Lock()
	defer this.Unlock()
	delete(this.data, key)
}
