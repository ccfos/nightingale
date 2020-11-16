// Copyright (c) 2016 Uber Technologies, Inc.
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

// Package watch provides utilities for watching resources for changes.
package watch

import (
	"errors"
	"sync"

	xclose "github.com/m3db/m3/src/x/close"
)

var errClosed = errors.New("closed")

type closer func()

// Updatable can be updated.
type Updatable interface {
	xclose.SimpleCloser

	// C returns the notification channel for updates.
	C() <-chan struct{}
}

// Watch watches a Watchable instance, can get notification when the Watchable updates.
type Watch interface {
	Updatable

	// Get returns the latest value of the Watchable instance.
	Get() interface{}
}

// Watchable can be watched
type Watchable interface {
	xclose.SimpleCloser

	// IsClosed returns true if the Watchable is closed
	IsClosed() bool
	// Get returns the latest value
	Get() interface{}
	// Watch returns the value and a Watch that will be notified on updates
	Watch() (interface{}, Watch, error)
	// NumWatches returns the number of watches on the Watchable
	NumWatches() int
	// Update sets the value and notify Watches
	Update(interface{}) error
}

// NewWatchable returns a Watchable
func NewWatchable() Watchable {
	return &watchable{}
}

type watchable struct {
	sync.RWMutex

	value  interface{}
	active []chan struct{}
	closed bool
}

func (w *watchable) Get() interface{} {
	w.RLock()
	v := w.value
	w.RUnlock()
	return v
}

func (w *watchable) Watch() (interface{}, Watch, error) {
	w.Lock()

	if w.closed {
		w.Unlock()
		return nil, nil, errClosed
	}

	c := make(chan struct{}, 1)
	notify := w.value != nil
	w.active = append(w.active, c)
	w.Unlock()

	if notify {
		select {
		case c <- struct{}{}:
		default:
		}
	}

	closeFn := w.closeFunc(c)
	watch := &watch{o: w, c: c, closeFn: closeFn}
	return w.Get(), watch, nil
}

func (w *watchable) Update(v interface{}) error {
	w.Lock()
	defer w.Unlock()

	if w.closed {
		return errClosed
	}

	w.value = v

	for _, s := range w.active {
		select {
		case s <- struct{}{}:
		default:
		}
	}

	return nil
}

func (w *watchable) NumWatches() int {
	w.RLock()
	l := len(w.active)
	w.RUnlock()

	return l
}

func (w *watchable) IsClosed() bool {
	w.RLock()
	c := w.closed
	w.RUnlock()

	return c
}

func (w *watchable) Close() {
	w.Lock()
	defer w.Unlock()

	if w.closed {
		return
	}

	w.closed = true

	for _, ch := range w.active {
		close(ch)
	}
	w.active = nil
}

func (w *watchable) closeFunc(c chan struct{}) closer {
	return func() {
		w.Lock()
		defer w.Unlock()

		if w.closed {
			return
		}

		close(c)

		for i := 0; i < len(w.active); i++ {
			if w.active[i] == c {
				w.active = append(w.active[:i], w.active[i+1:]...)
				break
			}
		}
	}
}

type watch struct {
	sync.Mutex

	o       Watchable
	c       <-chan struct{}
	closed  bool
	closeFn closer
}

func (w *watch) C() <-chan struct{} {
	return w.c
}

func (w *watch) Get() interface{} {
	return w.o.Get()
}

func (w *watch) Close() {
	w.Lock()
	defer w.Unlock()

	if w.closed {
		return
	}

	w.closed = true

	if w.closeFn != nil {
		w.closeFn()
	}
}
