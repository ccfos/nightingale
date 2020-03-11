package mcache

import "sync"

// MaskCacheMap 给alarm用，判断告警事件是否被屏蔽
// key是'${metric}#${endpoint}，value是list，
// 每一条是屏蔽策略中配置的tags，比如service=x,module=y
type MaskCacheMap struct {
	sync.RWMutex
	Data map[string][]string
}

var MaskCache *MaskCacheMap

func NewMaskCache() *MaskCacheMap {
	return &MaskCacheMap{
		Data: make(map[string][]string),
	}
}

func (mc *MaskCacheMap) SetAll(m map[string][]string) {
	mc.Lock()
	mc.Data = m
	mc.Unlock()
}

func (mc *MaskCacheMap) GetByKey(key string) ([]string, bool) {
	mc.RLock()
	value, exists := mc.Data[key]
	mc.RUnlock()
	return value, exists
}
