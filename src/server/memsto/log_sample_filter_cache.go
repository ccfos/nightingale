package memsto

import (
	"sync"
)

type LogSampleCacheType struct {
	sync.RWMutex
	m map[string]map[string]struct{} // map[labelName]map[labelValue]struct{}
}

var LogSampleCache = LogSampleCacheType{
	m: make(map[string]map[string]struct{}),
}

func (l *LogSampleCacheType) Set(m map[string][]string) {
	l.Lock()
	for k, v := range m {
		l.m[k] = make(map[string]struct{})
		for _, vv := range v {
			l.m[k][vv] = struct{}{}
		}
	}
	l.Unlock()
}

func (l *LogSampleCacheType) Get() map[string]map[string]struct{} {
	l.RLock()
	defer l.RUnlock()

	return l.m
}

func (l *LogSampleCacheType) Clean() {
	l.Lock()
	l.m = make(map[string]map[string]struct{})
	l.Unlock()
}

func (l *LogSampleCacheType) Len() int {
	l.RLock()
	defer l.RUnlock()
	return len(l.m)
}
