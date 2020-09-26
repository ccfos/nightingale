package acache

import "sync"

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

func (this *MaskCacheMap) SetAll(m map[string][]string) {
	this.Lock()
	defer this.Unlock()
	this.Data = m
}

func (this *MaskCacheMap) GetByKey(key string) ([]string, bool) {
	this.RLock()
	defer this.RUnlock()

	value, exists := this.Data[key]

	return value, exists
}
