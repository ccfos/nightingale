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
	"time"

	"github.com/m3db/m3/src/x/instrument"
)

const (
	defaultInitWatchTimeout = 10 * time.Second
)

// Options provide a set of value options.
type Options interface {
	// SetInstrumentOptions sets the instrument options.
	SetInstrumentOptions(value instrument.Options) Options

	// InstrumentOptions returns the instrument options.
	InstrumentOptions() instrument.Options

	// SetInitWatchTimeout sets the initial watch timeout.
	SetInitWatchTimeout(value time.Duration) Options

	// InitWatchTimeout returns the initial watch timeout.
	InitWatchTimeout() time.Duration

	// SetNewUpdatableFn sets the new updatable function.
	SetNewUpdatableFn(value NewUpdatableFn) Options

	// NewUpdatableFn returns the new updatable function.
	NewUpdatableFn() NewUpdatableFn

	// SetGetUpdateFn sets the get function.
	SetGetUpdateFn(value GetUpdateFn) Options

	// GetUpdateFn returns the get function.
	GetUpdateFn() GetUpdateFn

	// SetProcessFn sets the process function.
	SetProcessFn(value ProcessFn) Options

	// ProcessFn returns the process function.
	ProcessFn() ProcessFn

	// Key returns the key for the watch.
	Key() string

	// SetKey sets the key for the watch.
	SetKey(key string) Options
}

type options struct {
	instrumentOpts   instrument.Options
	initWatchTimeout time.Duration
	newUpdatableFn   NewUpdatableFn
	getUpdateFn      GetUpdateFn
	processFn        ProcessFn
	key              string
}

// NewOptions creates a new set of options.
func NewOptions() Options {
	return &options{
		instrumentOpts:   instrument.NewOptions(),
		initWatchTimeout: defaultInitWatchTimeout,
	}
}

func (o *options) SetInstrumentOptions(value instrument.Options) Options {
	opts := *o
	opts.instrumentOpts = value
	return &opts
}

func (o *options) InstrumentOptions() instrument.Options {
	return o.instrumentOpts
}

func (o *options) SetInitWatchTimeout(value time.Duration) Options {
	opts := *o
	opts.initWatchTimeout = value
	return &opts
}

func (o *options) InitWatchTimeout() time.Duration {
	return o.initWatchTimeout
}

func (o *options) SetNewUpdatableFn(value NewUpdatableFn) Options {
	opts := *o
	opts.newUpdatableFn = value
	return &opts
}

func (o *options) NewUpdatableFn() NewUpdatableFn {
	return o.newUpdatableFn
}

func (o *options) SetGetUpdateFn(value GetUpdateFn) Options {
	opts := *o
	opts.getUpdateFn = value
	return &opts
}

func (o *options) GetUpdateFn() GetUpdateFn {
	return o.getUpdateFn
}

func (o *options) SetProcessFn(value ProcessFn) Options {
	opts := *o
	opts.processFn = value
	return &opts
}

func (o *options) ProcessFn() ProcessFn {
	return o.processFn
}

func (o *options) Key() string {
	return o.key
}

func (o *options) SetKey(key string) Options {
	opts := *o
	opts.key = key
	return &opts
}
