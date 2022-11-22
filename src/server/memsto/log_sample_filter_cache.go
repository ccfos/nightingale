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

func (l *LogSampleCacheType) Exists(key, value string) bool {
	l.RLock()
	defer l.RUnlock()

	// * 匹配所有
	_, exists := l.m["*"]
	if exists {
		return true
	}

	valueMap, exists := l.m[key]
	if !exists {
		return false
	}

	// * 匹配所有
	_, exists = valueMap["*"]
	if exists {
		return true
	}

	_, exists = valueMap[value]
	return exists
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
