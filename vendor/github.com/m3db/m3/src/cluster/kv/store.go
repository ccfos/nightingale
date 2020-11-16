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

package kv

import (
	"errors"

	xwatch "github.com/m3db/m3/src/x/watch"
)

var (
	errEmptyZone        = errors.New("empty kv zone")
	errEmptyEnvironment = errors.New("empty kv environment")
	errEmptyNamespace   = errors.New("empty kv namespace")
)

type valueWatch struct {
	w xwatch.Watch
}

// newValueWatch creates a new ValueWatch
func newValueWatch(w xwatch.Watch) ValueWatch {
	return &valueWatch{w: w}
}

func (v *valueWatch) Close() {
	v.w.Close()
}

func (v *valueWatch) C() <-chan struct{} {
	return v.w.C()
}

func (v *valueWatch) Get() Value {
	return valueFromWatch(v.w.Get())
}

type valueWatchable struct {
	w xwatch.Watchable
}

// NewValueWatchable creates a new ValueWatchable
func NewValueWatchable() ValueWatchable {
	return &valueWatchable{w: xwatch.NewWatchable()}
}

func (w *valueWatchable) IsClosed() bool {
	return w.w.IsClosed()
}

func (w *valueWatchable) Close() {
	w.w.Close()
}

func (w *valueWatchable) Get() Value {
	return valueFromWatch(w.w.Get())
}

func (w *valueWatchable) Watch() (Value, ValueWatch, error) {
	value, watch, err := w.w.Watch()
	if err != nil {
		return nil, nil, err
	}

	return valueFromWatch(value), newValueWatch(watch), nil
}

func (w *valueWatchable) NumWatches() int {
	return w.w.NumWatches()
}

func (w *valueWatchable) Update(v Value) error {
	return w.w.Update(v)
}

func valueFromWatch(value interface{}) Value {
	if value != nil {
		return value.(Value)
	}

	return nil
}

type overrideOptions struct {
	zone      string
	env       string
	namespace string
}

// NewOverrideOptions creates a new kv Options.
func NewOverrideOptions() OverrideOptions {
	return overrideOptions{}
}

func (opts overrideOptions) Zone() string {
	return opts.zone
}

func (opts overrideOptions) SetZone(value string) OverrideOptions {
	opts.zone = value
	return opts
}

func (opts overrideOptions) Environment() string {
	return opts.env
}

func (opts overrideOptions) SetEnvironment(env string) OverrideOptions {
	opts.env = env
	return opts
}

func (opts overrideOptions) Namespace() string {
	return opts.namespace
}

func (opts overrideOptions) SetNamespace(namespace string) OverrideOptions {
	opts.namespace = namespace
	return opts
}

func (opts overrideOptions) Validate() error {
	if opts.zone == "" {
		return errEmptyZone
	}
	if opts.env == "" {
		return errEmptyEnvironment
	}
	if opts.namespace == "" {
		return errEmptyNamespace
	}
	return nil
}
