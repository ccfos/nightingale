package cache

import (
	"sync"

	"github.com/didi/nightingale/v4/src/models"
)

type AlarmStraCacheMap struct {
	sync.RWMutex
	Data map[int64]*models.Stra
}

var AlarmStraCache *AlarmStraCacheMap

func NewAlarmStraCache() *AlarmStraCacheMap {
	return &AlarmStraCacheMap{
		Data: make(map[int64]*models.Stra),
	}
}

func (this *AlarmStraCacheMap) SetAll(m map[int64]*models.Stra) {
	this.Lock()
	defer this.Unlock()
	this.Data = m
}

func (this *AlarmStraCacheMap) GetById(id int64) (*models.Stra, bool) {
	this.RLock()
	defer this.RUnlock()

	value, exists := this.Data[id]

	return value, exists
}
