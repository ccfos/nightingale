package process

import (
	"sync"

	"github.com/ccfos/nightingale/v6/models"
)

type AlertCurEventMap struct {
	sync.RWMutex
	Data map[string]*models.AlertCurEvent
}

func NewAlertCurEventMap(data map[string]*models.AlertCurEvent) *AlertCurEventMap {
	if data == nil {
		return &AlertCurEventMap{
			Data: make(map[string]*models.AlertCurEvent),
		}
	}
	return &AlertCurEventMap{
		Data: data,
	}
}

func (a *AlertCurEventMap) SetAll(data map[string]*models.AlertCurEvent) {
	a.Lock()
	defer a.Unlock()
	a.Data = data
}

func (a *AlertCurEventMap) Set(key string, value *models.AlertCurEvent) {
	a.Lock()
	defer a.Unlock()
	a.Data[key] = value
}

func (a *AlertCurEventMap) Get(key string) (*models.AlertCurEvent, bool) {
	a.RLock()
	defer a.RUnlock()
	event, exists := a.Data[key]
	return event, exists
}

func (a *AlertCurEventMap) UpdateLastEvalTime(key string, lastEvalTime int64) {
	a.Lock()
	defer a.Unlock()
	event, exists := a.Data[key]
	if !exists {
		return
	}
	event.LastEvalTime = lastEvalTime
}

func (a *AlertCurEventMap) Delete(key string) {
	a.Lock()
	defer a.Unlock()
	delete(a.Data, key)
}

func (a *AlertCurEventMap) Keys() []string {
	a.RLock()
	defer a.RUnlock()
	keys := make([]string, 0, len(a.Data))
	for k := range a.Data {
		keys = append(keys, k)
	}
	return keys
}

func (a *AlertCurEventMap) GetAll() map[string]*models.AlertCurEvent {
	a.RLock()
	defer a.RUnlock()
	return a.Data
}
