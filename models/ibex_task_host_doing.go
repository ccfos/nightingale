package models

import (
	"encoding/json"
	"fmt"
	"sync"
)

type TaskHostDoing struct {
	Id             int64  `gorm:"column:id;index"`
	Host           string `gorm:"column:host;size:128;not null;index"`
	Clock          int64  `gorm:"column:clock;not null;default:0"`
	Action         string `gorm:"column:action;size:16;not null"`
	AlertTriggered bool   `gorm:"-"`
}

func (TaskHostDoing) TableName() string {
	return "task_host_doing"
}

func (doing *TaskHostDoing) MarshalBinary() ([]byte, error) {
	return json.Marshal(doing)
}

func (doing *TaskHostDoing) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, doing)
}

func hostDoingCacheKey(id int64, host string) string {
	return fmt.Sprintf("%s:%d", host, id)
}

var (
	doingLock sync.RWMutex
	doingMaps map[string][]TaskHostDoing
)

func SetDoingCache(v map[string][]TaskHostDoing) {
	doingLock.Lock()
	doingMaps = v
	doingLock.Unlock()
}

func GetDoingCache(host string) []TaskHostDoing {
	doingLock.RLock()
	defer doingLock.RUnlock()

	return doingMaps[host]
}

func CheckExistAndEdgeAlertTriggered(host string, id int64) (exist, isAlertTriggered bool) {
	doingLock.RLock()
	defer doingLock.RUnlock()

	doings := doingMaps[host]
	for _, doing := range doings {
		if doing.Id == id {
			exist = true
			isAlertTriggered = doing.AlertTriggered
			return
		}
	}

	return false, false
}
