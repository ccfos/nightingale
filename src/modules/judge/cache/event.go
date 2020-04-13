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

func (s *SafeEventMap) Get(key string) (*dataobj.Event, bool) {
	s.RLock()
	defer s.RUnlock()
	event, exists := s.M[key]
	return event, exists
}

func (s *SafeEventMap) Set(key string, event *dataobj.Event) {
	s.Lock()
	defer s.Unlock()
	s.M[key] = event
}
