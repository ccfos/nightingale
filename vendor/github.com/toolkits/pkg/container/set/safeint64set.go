package set

import (
	"fmt"
	"sync"
)

type SafeInt64Set struct {
	sync.RWMutex
	M map[int64]struct{}
}

func NewSafeInt64Set() *SafeInt64Set {
	return &SafeInt64Set{M: make(map[int64]struct{})}
}

func (this *SafeInt64Set) String() string {
	s := this.Slice()
	return fmt.Sprint(s)
}

func (this *SafeInt64Set) Add(item int64) *SafeInt64Set {
	if this.Contains(item) {
		return this
	}

	this.Lock()
	this.M[item] = struct{}{}
	this.Unlock()
	return this
}

func (this *SafeInt64Set) Contains(item int64) bool {
	this.RLock()
	_, exists := this.M[item]
	this.RUnlock()
	return exists
}

func (this *SafeInt64Set) Adds(items []int64) *SafeInt64Set {
	count := len(items)
	if count == 0 {
		return this
	}

	todo := make([]int64, 0, count)
	this.RLock()
	for i := 0; i < count; i++ {
		_, exists := this.M[items[i]]
		if exists {
			continue
		}

		todo = append(todo, items[i])
	}
	this.RUnlock()

	count = len(todo)
	if count == 0 {
		return this
	}

	this.Lock()
	for i := 0; i < count; i++ {
		this.M[todo[i]] = struct{}{}
	}
	this.Unlock()
	return this
}

func (this *SafeInt64Set) Size() int {
	this.RLock()
	l := len(this.M)
	this.RUnlock()
	return l
}

func (this *SafeInt64Set) Clear() {
	this.Lock()
	this.M = make(map[int64]struct{})
	this.Unlock()
}

func (this *SafeInt64Set) Slice() []int64 {
	this.RLock()
	ret := make([]int64, len(this.M))
	i := 0
	for item := range this.M {
		ret[i] = item
		i++
	}

	this.RUnlock()
	return ret
}
