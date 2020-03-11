package httplib

// 这个暂时用不到

import (
	"sync"
	"time"
)

// DeadCache 死掉的实例缓存
type DeadCache struct {
	sync.RWMutex
	Data     map[string]int64
	Duration time.Duration
}

// NewDeadCache 实例化DeadCache，每个HTTPServer只对应一个DeadCache
func NewDeadCache(duration time.Duration) DeadCache {
	deadCache := DeadCache{
		Duration: duration,
		Data:     make(map[string]int64),
	}

	go deadCache.Clean()
	return deadCache
}

// Set 放置一个挂掉的实例
func (dc *DeadCache) Set(key string) {
	dc.Lock()
	defer dc.Unlock()
	dc.Data[key] = time.Now().Unix()
}

// Exists 检查某个实例是否存在
func (dc *DeadCache) Exists(key string) bool {
	dc.RLock()
	defer dc.RUnlock()
	_, exists := dc.Data[key]
	return exists
}

// Size 计算所有挂掉的实例的个数
func (dc *DeadCache) Size() int {
	dc.RLock()
	defer dc.RUnlock()
	return len(dc.Data)
}

// Clean 缓存清理方法，需要外层起一个goroutine来调用
func (dc *DeadCache) Clean() []string {
	t1 := time.NewTicker(1 * time.Second)

	for {
		<-t1.C

		now := time.Now().Unix()
		dc.Lock()
		for instance, timestamp := range dc.Data {
			if now-timestamp > int64(dc.Duration) {
				delete(dc.Data, instance)
			}
		}
		dc.Unlock()
	}
}
