package cache

import (
	"sync"

	"github.com/didi/nightingale/src/dataobj"
)

type SafeEventMap struct {
	sync.RWMutex
	M map[string]*dataobj.Event
}

var (
	LastEvents = &SafeEventMap{M: make(map[string]*dataobj.Event)}
)

func (this *SafeEventMap) Get(key string) (*dataobj.Event, bool) {
	this.RLock()
	defer this.RUnlock()
	event, exists := this.M[key]
	return event, exists
}

func (this *SafeEventMap) Set(key string, event *dataobj.Event) {
	this.Lock()
	defer this.Unlock()
	this.M[key] = event
}
