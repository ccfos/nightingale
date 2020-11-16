// Copyright (c) 2020 Uber Technologies, Inc.
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

package namespace

import (
	"sync"

	xclose "github.com/m3db/m3/src/x/close"
	"github.com/m3db/m3/src/x/watch"
)

const (
	defaultWriteIndexingPerCPUConcurrency = 0.75
	defaultFlushIndexingPerCPUConcurrency = 0.25
)

// RuntimeOptions is a set of runtime options that can
// be set per namespace.
type RuntimeOptions interface {
	// IsDefault returns whether the runtime options are purely defaults
	// with no values explicitly set.
	IsDefault() bool

	// Equal will return whether it's equal to another runtime options.
	Equal(other RuntimeOptions) bool

	// SetWriteIndexingPerCPUConcurrency sets the write
	// indexing per CPU concurrency.
	SetWriteIndexingPerCPUConcurrency(value *float64) RuntimeOptions

	// WriteIndexingPerCPUConcurrency returns the write
	// indexing per CPU concurrency.
	WriteIndexingPerCPUConcurrency() *float64

	// WriteIndexingPerCPUConcurrencyOrDefault returns the write
	// indexing per CPU concurrency or default.
	WriteIndexingPerCPUConcurrencyOrDefault() float64

	// SetFlushIndexingPerCPUConcurrency sets the flush
	// indexing per CPU concurrency.
	SetFlushIndexingPerCPUConcurrency(value *float64) RuntimeOptions

	// FlushIndexingPerCPUConcurrency returns the flush
	// indexing per CPU concurrency.
	FlushIndexingPerCPUConcurrency() *float64

	// FlushIndexingPerCPUConcurrencyOrDefault returns the flush
	// indexing per CPU concurrency.
	FlushIndexingPerCPUConcurrencyOrDefault() float64
}

// RuntimeOptionsManagerRegistry is a registry of runtime options managers.
type RuntimeOptionsManagerRegistry interface {
	// RuntimeOptionsManager returns a namespace runtime options manager
	// for the given namespace.
	RuntimeOptionsManager(namespace string) RuntimeOptionsManager

	// Close closes the watcher and all descendent watches.
	Close()
}

// RuntimeOptionsManager is a runtime options manager.
type RuntimeOptionsManager interface {
	// Update updates the current runtime options.
	Update(value RuntimeOptions) error

	// Get returns the current values.
	Get() RuntimeOptions

	// RegisterListener registers a listener for updates to runtime options,
	// it will synchronously call back the listener when this method is called
	// to deliver the current set of runtime options.
	RegisterListener(l RuntimeOptionsListener) xclose.SimpleCloser

	// Close closes the watcher and all descendent watches.
	Close()
}

// RuntimeOptionsListener listens for updates to runtime options.
type RuntimeOptionsListener interface {
	// SetNamespaceRuntimeOptions is called when the listener is registered
	// and when any updates occurred passing the new runtime options.
	SetNamespaceRuntimeOptions(value RuntimeOptions)
}

// runtimeOptions should always use pointer value types for it's options
// and provide a "ValueOrDefault()" method so that we can be sure whether
// the options are all defaults or not with the "IsDefault" method.
type runtimeOptions struct {
	writeIndexingPerCPUConcurrency *float64
	flushIndexingPerCPUConcurrency *float64
}

// NewRuntimeOptions returns a new namespace runtime options.
func NewRuntimeOptions() RuntimeOptions {
	return newRuntimeOptions()
}

func newRuntimeOptions() *runtimeOptions {
	return &runtimeOptions{}
}

func (o *runtimeOptions) IsDefault() bool {
	defaults := newRuntimeOptions()
	return *o == *defaults
}

func (o *runtimeOptions) Equal(other RuntimeOptions) bool {
	return o.writeIndexingPerCPUConcurrency == other.WriteIndexingPerCPUConcurrency() &&
		o.flushIndexingPerCPUConcurrency == other.FlushIndexingPerCPUConcurrency()
}

func (o *runtimeOptions) SetWriteIndexingPerCPUConcurrency(value *float64) RuntimeOptions {
	opts := *o
	opts.writeIndexingPerCPUConcurrency = value
	return &opts
}

func (o *runtimeOptions) WriteIndexingPerCPUConcurrency() *float64 {
	return o.writeIndexingPerCPUConcurrency
}

func (o *runtimeOptions) WriteIndexingPerCPUConcurrencyOrDefault() float64 {
	value := o.writeIndexingPerCPUConcurrency
	if value == nil {
		return defaultWriteIndexingPerCPUConcurrency
	}
	return *value
}

func (o *runtimeOptions) SetFlushIndexingPerCPUConcurrency(value *float64) RuntimeOptions {
	opts := *o
	opts.flushIndexingPerCPUConcurrency = value
	return &opts
}

func (o *runtimeOptions) FlushIndexingPerCPUConcurrency() *float64 {
	return o.flushIndexingPerCPUConcurrency
}

func (o *runtimeOptions) FlushIndexingPerCPUConcurrencyOrDefault() float64 {
	value := o.flushIndexingPerCPUConcurrency
	if value == nil {
		return defaultFlushIndexingPerCPUConcurrency
	}
	return *value
}

type runtimeOptionsManagerRegistry struct {
	sync.RWMutex
	managers map[string]RuntimeOptionsManager
}

// NewRuntimeOptionsManagerRegistry returns a new runtime options
// manager registry.
func NewRuntimeOptionsManagerRegistry() RuntimeOptionsManagerRegistry {
	return &runtimeOptionsManagerRegistry{
		managers: make(map[string]RuntimeOptionsManager),
	}
}

func (r *runtimeOptionsManagerRegistry) RuntimeOptionsManager(
	namespace string,
) RuntimeOptionsManager {
	r.Lock()
	defer r.Unlock()

	manager, ok := r.managers[namespace]
	if !ok {
		manager = NewRuntimeOptionsManager(namespace)
		r.managers[namespace] = manager
	}

	return manager
}

func (r *runtimeOptionsManagerRegistry) Close() {
	r.Lock()
	defer r.Unlock()

	for k, v := range r.managers {
		v.Close()
		delete(r.managers, k)
	}
}

type runtimeOptionsManager struct {
	namespace string
	watchable watch.Watchable
}

// NewRuntimeOptionsManager returns a new runtime options manager.
func NewRuntimeOptionsManager(namespace string) RuntimeOptionsManager {
	watchable := watch.NewWatchable()
	watchable.Update(NewRuntimeOptions())
	return &runtimeOptionsManager{
		namespace: namespace,
		watchable: watchable,
	}
}

func (w *runtimeOptionsManager) Update(value RuntimeOptions) error {
	w.watchable.Update(value)
	return nil
}

func (w *runtimeOptionsManager) Get() RuntimeOptions {
	return w.watchable.Get().(RuntimeOptions)
}

func (w *runtimeOptionsManager) RegisterListener(
	listener RuntimeOptionsListener,
) xclose.SimpleCloser {
	_, watch, _ := w.watchable.Watch()

	// We always initialize the watchable so always read
	// the first notification value
	<-watch.C()

	// Deliver the current runtime options
	listener.SetNamespaceRuntimeOptions(watch.Get().(RuntimeOptions))

	// Spawn a new goroutine that will terminate when the
	// watchable terminates on the close of the runtime options manager
	go func() {
		for range watch.C() {
			listener.SetNamespaceRuntimeOptions(watch.Get().(RuntimeOptions))
		}
	}()

	return watch
}

func (w *runtimeOptionsManager) Close() {
	w.watchable.Close()
}
