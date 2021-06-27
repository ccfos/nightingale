package cache

import (
	"sync"
)

type SafeDoubleMap struct {
	sync.RWMutex
	M map[string]map[string]struct{}
}

// res_ident -> classpath_path -> struct{}{}
var ResClasspath = &SafeDoubleMap{M: make(map[string]map[string]struct{})}

func (s *SafeDoubleMap) GetKeys() []string {
	s.RLock()
	defer s.RUnlock()

	keys := make([]string, 0, len(s.M))
	for key := range s.M {
		keys = append(keys, key)
	}

	return keys
}

func (s *SafeDoubleMap) GetValues(key string) []string {
	s.RLock()
	defer s.RUnlock()

	valueMap, exists := s.M[key]
	if !exists {
		return []string{}
	}

	values := make([]string, 0, len(valueMap))

	for value := range valueMap {
		values = append(values, value)
	}

	return values
}

func (s *SafeDoubleMap) Exists(key string, value string) bool {
	s.RLock()
	defer s.RUnlock()

	if _, exists := s.M[key]; !exists {
		return false
	}

	if _, exists := s.M[key][value]; !exists {
		return false
	}

	return true
}

func (s *SafeDoubleMap) Set(key string, value string) {
	s.Lock()
	defer s.Unlock()

	if _, exists := s.M[key]; !exists {
		s.M[key] = make(map[string]struct{})
	}

	s.M[key][value] = struct{}{}
}

func (s *SafeDoubleMap) SetAll(data map[string]map[string]struct{}) {
	s.Lock()
	defer s.Unlock()

	s.M = data
}
