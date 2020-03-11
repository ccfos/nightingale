package index

import (
	"reflect"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/tsdb/utils"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/toolkits/pkg/concurrent/semaphore"
	"github.com/toolkits/pkg/logger"
)

type IndexSection struct {
	ActiveDuration  int64  `yaml:"activeDuration"`  //内存索引保留时间
	RebuildInterval int64  `yaml:"rebuildInterval"` //索引重建周期
	HbsMod          string `yaml:"hbsMod"`
}

//重建索引全局锁
var UpdateIndexLock = semaphore.NewSemaphore(1)
var Config IndexSection

const INDEX_SHARD = 256

var IndexedItemCacheBigMap = make([]*IndexCacheBase, INDEX_SHARD)
var UnIndexedItemCacheBigMap = make([]*IndexCacheBase, INDEX_SHARD)

// 初始化索引功能模块
func Init(cfg IndexSection) {
	Config = cfg
	for i := 0; i < INDEX_SHARD; i++ {
		IndexedItemCacheBigMap[i] = NewIndexCacheBase(DefaultMaxCacheSize)
		UnIndexedItemCacheBigMap[i] = NewIndexCacheBase(DefaultMaxCacheSize)
	}

	go GetIndexLoop()
	go StartIndexUpdateIncrTask()
	go StartUpdateIndexTask()
	logger.Info("index.Start ok")
}

func GetItemFronIndex(hash interface{}) *dataobj.TsdbItem {
	switch hash.(type) {
	case uint64:
		indexedItemCache := IndexedItemCacheBigMap[hash.(uint64)%INDEX_SHARD]
		return indexedItemCache.Get(hash)
	case string:
		indexedItemCache := IndexedItemCacheBigMap[utils.HashKey(hash.(string))%INDEX_SHARD]
		return indexedItemCache.Get(hash)
	}

	return nil
}

func DeleteItemFronIndex(hash interface{}) *dataobj.TsdbItem {
	switch hash.(type) {
	case uint64:
		indexedItemCache := IndexedItemCacheBigMap[hash.(uint64)%INDEX_SHARD]
		indexedItemCache.Remove(hash)
	case string:
		indexedItemCache := IndexedItemCacheBigMap[utils.HashKey(hash.(string))%INDEX_SHARD]
		indexedItemCache.Remove(hash)
	}

	return nil
}

// index收到一条新上报的监控数据,尝试用于增量更新索引
func ReceiveItem(item *dataobj.TsdbItem, hash interface{}) {
	if item == nil {
		return
	}
	var indexedItemCache *IndexCacheBase
	var unIndexedItemCache *IndexCacheBase

	switch hash.(type) {
	case uint64:
		indexedItemCache = IndexedItemCacheBigMap[int(hash.(uint64)%INDEX_SHARD)]
		unIndexedItemCache = UnIndexedItemCacheBigMap[int(hash.(uint64)%INDEX_SHARD)]
	case string:
		indexedItemCache = IndexedItemCacheBigMap[int(hashKey(hash.(string))%INDEX_SHARD)]
		unIndexedItemCache = UnIndexedItemCacheBigMap[int(hashKey(hash.(string))%INDEX_SHARD)]
	default:
		logger.Error("undefined hash type", hash)
		stats.Counter.Set("index.in.err", 1)

		return
	}
	if indexedItemCache == nil {
		stats.Counter.Set("index.in.err", 1)
		logger.Error("indexedItemCache: ", reflect.TypeOf(hash), hash)
	}
	// 已上报过的数据
	stats.Counter.Set("index.in", 1)
	if indexedItemCache.ContainsKey(hash) {
		indexedItemCache.Put(hash, item)
		return
	}
	stats.Counter.Set("index.incr.in", 1)
	// 缓存未命中, 放入增量更新队列
	unIndexedItemCache.Put(hash, item)
	indexedItemCache.Put(hash, item)
}

func hashKey(key string) uint32 {
	hash := uint32(2166136261)
	const prime32 = uint32(16777619)
	for i := 0; i < len(key); i++ {
		hash *= prime32
		hash ^= uint32(key[i])
	}
	return hash
}
