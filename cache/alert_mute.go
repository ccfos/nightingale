package cache

import (
	"regexp"
	"sync"
)

type AlertMuteMap struct {
	sync.RWMutex
	Data map[string][]Filter
}
type Filter struct {
	ClasspathPrefix string
	ResReg          *regexp.Regexp
	TagsMap         map[string]string
}

var AlertMute = &AlertMuteMap{Data: make(map[string][]Filter)}

func (a *AlertMuteMap) SetAll(m map[string][]Filter) {
	a.Lock()
	defer a.Unlock()
	a.Data = m
}

func (a *AlertMuteMap) GetByKey(key string) ([]Filter, bool) {
	a.RLock()
	defer a.RUnlock()

	value, exists := a.Data[key]

	return value, exists
}
