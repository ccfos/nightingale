package acache

import (
	"sync"

	"github.com/didi/nightingale/src/models"
)

type StraCacheMap struct {
	sync.RWMutex
	Data map[int64]*models.Stra
}

var StraCache *StraCacheMap

func NewStraCache() *StraCacheMap {
	return &StraCacheMap{
		Data: make(map[int64]*models.Stra),
	}
}

func (this *StraCacheMap) SetAll(m map[int64]*models.Stra) {
	this.Lock()
	defer this.Unlock()
	this.Data = m
}

func (this *StraCacheMap) GetById(id int64) (*models.Stra, bool) {
	this.RLock()
	defer this.RUnlock()

	value, exists := this.Data[id]

	return value, exists
}
