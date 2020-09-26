package set

import (
	"sync"
)

type SafeSet struct {
	sync.RWMutex
	M map[string]bool
}

func NewSafeSet() *SafeSet {
	return &SafeSet{
		M: make(map[string]bool),
	}
}

func (this *SafeSet) Add(key string) {
	this.Lock()
	this.M[key] = true
	this.Unlock()
}

func (this *SafeSet) Remove(key string) {
	this.Lock()
	delete(this.M, key)
	this.Unlock()
}

func (this *SafeSet) Clear() {
	this.Lock()
	this.M = make(map[string]bool)
	this.Unlock()
}

func (this *SafeSet) Contains(key string) bool {
	this.RLock()
	_, exists := this.M[key]
	this.RUnlock()
	return exists
}

func (this *SafeSet) Size() int {
	this.RLock()
	len := len(this.M)
	this.RUnlock()
	return len
}

func (this *SafeSet) ToSlice() []string {
	this.RLock()
	defer this.RUnlock()

	count := len(this.M)
	if count == 0 {
		return []string{}
	}

	r := []string{}
	for key := range this.M {
		r = append(r, key)
	}

	return r
}
