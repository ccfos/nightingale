package cache

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/didi/nightingale/src/modules/tsdb/utils"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/toolkits/pkg/logger"
)

type CacheSection struct {
	KeepMinutes      int `yaml:"keepMinutes"`
	SpanInSeconds    int `yaml:"spanInSeconds"`
	NumOfChunks      int `yaml:"numOfChunks"`
	DoCleanInMinutes int `yaml:"doCleanInMinutes"`
	FlushDiskStepMs  int `yaml:"flushDiskStepMs"`
}

const SHARD_COUNT = 256

var (
	Caches caches
	Config CacheSection
)

var (
	TotalCount int64
	cleaning   bool
)

type (
	caches []*cache
)

type cache struct {
	Items map[string]*CS // [counter]ts,value
	sync.RWMutex
}

func Init(cfg CacheSection) {
	Config = cfg

	//根据内存保存曲线的时长，计算出需要几个chunk
	//如果内存保留2个小时数据，+1为了查询2个小时内的数据一定落在内存中
	Config.NumOfChunks = Config.KeepMinutes*60/Config.SpanInSeconds + 1

	InitCaches()
	go StartCleanup()
}

func InitCaches() {
	Caches = NewCaches()
}

func InitChunkSlot() {
	size := Config.SpanInSeconds * 1000 / Config.FlushDiskStepMs
	if size < 0 {
		log.Panicf("store.init, bad size %d\n", size)
	}

	ChunksSlots = &ChunksSlot{
		Data: make([]map[string][]*Chunk, size),
		Size: size,
	}
	for i := 0; i < size; i++ {
		ChunksSlots.Data[i] = make(map[string][]*Chunk)
	}
}

func NewCaches() caches {
	c := make(caches, SHARD_COUNT)
	for i := 0; i < SHARD_COUNT; i++ {
		c[i] = &cache{Items: make(map[string]*CS)}
	}
	return c
}

func StartCleanup() {
	cfg := Config
	t := time.NewTicker(time.Minute * time.Duration(cfg.DoCleanInMinutes))
	cleaning = false

	for {
		select {
		case <-t.C:
			if !cleaning {
				go Caches.Cleanup(cfg.KeepMinutes)
			} else {
				logger.Warning("cleanup() is working, may be it's too slow")
			}
		}
	}
}

func (c *caches) Push(seriesID string, ts int64, value float64) error {
	shard := c.getShard(seriesID)
	existC, exist := Caches.exist(seriesID)
	if exist {
		shard.Lock()
		err := existC.Push(seriesID, ts, value)
		shard.Unlock()
		return err
	}
	newC := Caches.create(seriesID)
	shard.Lock()
	err := newC.Push(seriesID, ts, value)
	shard.Unlock()

	return err
}

func (c *caches) Get(seriesID string, from, to int64) ([]Iter, error) {
	existC, exist := Caches.exist(seriesID)

	if !exist {
		return nil, fmt.Errorf("non series exist")
	}

	res := existC.Get(from, to)
	if res == nil {
		return nil, fmt.Errorf("non enough data")
	}

	return res, nil
}

func (c *caches) SetFlag(seriesID string, flag uint32) error {
	existC, exist := Caches.exist(seriesID)
	if !exist {
		return fmt.Errorf("non series exist")
	}
	existC.SetFlag(flag)
	return nil
}

func (c *caches) GetFlag(seriesID string) uint32 {
	existC, exist := Caches.exist(seriesID)
	if !exist {
		return 0
	}
	return existC.GetFlag()
}

func (c *caches) create(seriesID string) *CS {
	atomic.AddInt64(&TotalCount, 1)
	shard := c.getShard(seriesID)
	shard.Lock()
	newC := NewChunks(Config.NumOfChunks)
	shard.Items[seriesID] = newC
	shard.Unlock()

	return newC
}

func (c *caches) exist(seriesID string) (*CS, bool) {
	shard := c.getShard(seriesID)
	shard.RLock()
	existC, exist := shard.Items[seriesID]
	shard.RUnlock()

	return existC, exist
}

func (c *caches) GetCurrentChunk(seriesID string) (*Chunk, bool) {
	shard := c.getShard(seriesID)
	if shard == nil {
		return nil, false
	}
	shard.RLock()
	existC, exists := shard.Items[seriesID]
	shard.RUnlock()
	if exists {
		chunk := existC.GetChunk(existC.CurrentChunkPos)
		return chunk, exists
	}
	return nil, exists
}

func (c caches) Count() int64 {
	return atomic.LoadInt64(&TotalCount)
}

func (c caches) Remove(seriesID string) {
	atomic.AddInt64(&TotalCount, -1)
	shard := c.getShard(seriesID)
	shard.Lock()
	delete(shard.Items, seriesID)
	shard.Unlock()
}

func (c caches) Cleanup(expiresInMinutes int) {
	now := time.Now()
	done := make(chan struct{})
	var count int64
	cleaning = true
	defer func() { cleaning = false }()

	go func() {
		wg := sync.WaitGroup{}
		wg.Add(SHARD_COUNT)

		for _, shard := range c {
			go func(shard *cache) {
				shard.RLock()
				for key, chunks := range shard.Items {
					_, lastTs := chunks.GetInfoUnsafe()
					if int64(lastTs) < now.Unix()-60*int64(expiresInMinutes) {
						atomic.AddInt64(&count, 1)
						shard.RUnlock()
						c.Remove(key)
						stats.Counter.Set("series.delete", 1)
						shard.RLock()
					}
				}
				shard.RUnlock()
				wg.Done()
			}(shard)
		}
		wg.Wait()
		done <- struct{}{}
	}()

	<-done
	logger.Infof("cleanup %v Items, took %.2f ms\n", count, float64(time.Since(now).Nanoseconds())*1e-6)
}

func (c caches) getShard(key string) *cache {
	return c[utils.HashKey(key)%SHARD_COUNT]
}
