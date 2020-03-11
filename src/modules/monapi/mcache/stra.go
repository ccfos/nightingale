package mcache

import (
	"sync"

	"github.com/didi/nightingale/src/model"
)

// StraCacheMap 给alarm用
type StraCacheMap struct {
	sync.RWMutex
	Data map[int64]*model.Stra
}

var StraCache *StraCacheMap

func NewStraCache() *StraCacheMap {
	return &StraCacheMap{
		Data: make(map[int64]*model.Stra),
	}
}

func (sc *StraCacheMap) SetAll(m map[int64]*model.Stra) {
	sc.Lock()
	sc.Data = m
	sc.Unlock()
}

func (sc *StraCacheMap) GetById(id int64) (*model.Stra, bool) {
	sc.RLock()
	value, exists := sc.Data[id]
	sc.RUnlock()
	return value, exists
}
