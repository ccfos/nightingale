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

package services

import (
	"github.com/m3db/m3/src/x/watch"
)

// NewWatch creates a Watch.
func NewWatch(w watch.Watch) Watch {
	return &serviceWatch{value: w}
}

type serviceWatch struct {
	value watch.Watch
}

func (w *serviceWatch) Close() {
	w.value.Close()
}

func (w *serviceWatch) C() <-chan struct{} {
	return w.value.C()
}

func (w *serviceWatch) Get() Service {
	return w.value.Get().(Service)
}

type serviceWatchable interface {
	watches() int
	update(v Service)
	get() Service
	watch() (Service, Watch, error)
}

type watchable struct {
	value watch.Watchable
}

func newServiceWatchable() serviceWatchable {
	return &watchable{value: watch.NewWatchable()}
}

func (w *watchable) watches() int {
	return w.value.NumWatches()
}

func (w *watchable) update(v Service) {
	w.value.Update(v)
}

func (w *watchable) get() Service {
	return w.value.Get().(Service)
}

func (w *watchable) watch() (Service, Watch, error) {
	curr, newWatch, err := w.value.Watch()
	if err != nil {
		return nil, nil, err
	}
	return curr.(Service), NewWatch(newWatch), nil
}
