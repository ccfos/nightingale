// Copyright (c) 2018 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package watch

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

var (
	errInitWatchTimeout = errors.New("init watch timeout")
	errNilValue         = errors.New("nil kv value")
)

// Value is a resource that can be updated during runtime.
type Value interface {
	// Watch starts watching for value updates.
	Watch() error

	// Unwatch stops watching for value updates.
	Unwatch()
}

// NewUpdatableFn creates an updatable.
type NewUpdatableFn func() (Updatable, error)

// GetUpdateFn returns the latest value.
type GetUpdateFn func(updatable Updatable) (interface{}, error)

// ProcessFn processes an update.
type ProcessFn func(update interface{}) error

// processWithLockFn updates a value while holding a lock.
type processWithLockFn func(update interface{}) error

type valueStatus int

const (
	valueNotWatching valueStatus = iota
	valueWatching
)

type value struct {
	sync.RWMutex

	opts              Options
	log               *zap.Logger
	newUpdatableFn    NewUpdatableFn
	getUpdateFn       GetUpdateFn
	processFn         ProcessFn
	processWithLockFn processWithLockFn
	key               string

	updatable Updatable
	status    valueStatus
}

// NewValue creates a new value.
func NewValue(
	opts Options,
) Value {
	v := &value{
		opts:           opts,
		log:            opts.InstrumentOptions().Logger(),
		newUpdatableFn: opts.NewUpdatableFn(),
		getUpdateFn:    opts.GetUpdateFn(),
		processFn:      opts.ProcessFn(),
	}
	v.processWithLockFn = v.processWithLock
	return v
}

func (v *value) Watch() error {
	v.Lock()
	defer v.Unlock()

	if v.status == valueWatching {
		return nil
	}
	updatable, err := v.newUpdatableFn()
	if err != nil {
		return CreateWatchError{
			innerError: err,
			key:        v.opts.Key(),
		}
	}
	v.status = valueWatching
	v.updatable = updatable
	// NB(xichen): we want to start watching updates even though
	// we may fail to initialize the value temporarily (e.g., during
	// a network partition) so the value will be updated when the
	// error condition is resolved.
	defer func() { go v.watchUpdates(v.updatable) }()

	select {
	case <-v.updatable.C():
	case <-time.After(v.opts.InitWatchTimeout()):
		return InitValueError{
			innerError: errInitWatchTimeout,
			key:        v.opts.Key(),
		}
	}

	update, err := v.getUpdateFn(v.updatable)
	if err != nil {
		return InitValueError{
			innerError: err,
			key:        v.opts.Key(),
		}
	}

	if err = v.processWithLockFn(update); err != nil {
		return InitValueError{
			innerError: err,
			key:        v.opts.Key(),
		}
	}
	return nil
}

func (v *value) Unwatch() {
	v.Lock()
	defer v.Unlock()

	// If status is nil, it means we are not watching.
	if v.status == valueNotWatching {
		return
	}
	v.updatable.Close()
	v.status = valueNotWatching
	v.updatable = nil
}

func (v *value) watchUpdates(updatable Updatable) {
	for range updatable.C() {
		v.Lock()
		// If we are not watching, or we are watching with a different
		// watch because we stopped the current watch and started a new
		// one, return immediately.
		if v.status != valueWatching || v.updatable != updatable {
			v.Unlock()
			return
		}
		update, err := v.getUpdateFn(updatable)
		if err != nil {
			v.log.Error("error getting update",
				zap.String("key", v.opts.Key()),
				zap.Error(err))
			v.Unlock()
			continue
		}
		if err = v.processWithLockFn(update); err != nil {
			v.log.Error("error applying update",
				zap.String("key", v.opts.Key()),
				zap.Error(err))
		}
		v.Unlock()
	}
}

func (v *value) processWithLock(update interface{}) error {
	if update == nil {
		return errNilValue
	}
	return v.processFn(update)
}

// CreateWatchError is returned when encountering an error creating a watch.
type CreateWatchError struct {
	innerError error
	key        string
}

func (e CreateWatchError) Error() string {
	return fmt.Sprintf("create watch error (key='%s'): %v", e.key, e.innerError)
}

// InitValueError is returned when encountering an error when initializing a value.
type InitValueError struct {
	innerError error
	key        string
}

func (e InitValueError) Error() string {
	return fmt.Sprintf("initializing value error (key='%s'): %v", e.key, e.innerError)
}
