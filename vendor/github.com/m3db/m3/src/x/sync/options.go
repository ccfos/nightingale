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

package sync

import (
	"runtime"
	"time"

	"github.com/m3db/m3/src/x/instrument"
)

const (
	defaultKillWorkerProbability = 0.001
	defaultGrowOnDemand          = false
)

var (
	defaultNumShards = int64(runtime.NumCPU())
	defaultNowFn     = time.Now
)

// NowFn is a function that returns the current time.
type NowFn func() time.Time

// NewPooledWorkerPoolOptions returns a new PooledWorkerPoolOptions with default options
func NewPooledWorkerPoolOptions() PooledWorkerPoolOptions {
	return &pooledWorkerPoolOptions{
		growOnDemand:          defaultGrowOnDemand,
		numShards:             defaultNumShards,
		killWorkerProbability: defaultKillWorkerProbability,
		nowFn: defaultNowFn,
		iOpts: instrument.NewOptions(),
	}
}

type pooledWorkerPoolOptions struct {
	growOnDemand          bool
	numShards             int64
	killWorkerProbability float64
	nowFn                 NowFn
	iOpts                 instrument.Options
}

func (o *pooledWorkerPoolOptions) SetGrowOnDemand(value bool) PooledWorkerPoolOptions {
	opts := *o
	opts.growOnDemand = value
	return &opts
}

func (o *pooledWorkerPoolOptions) GrowOnDemand() bool {
	return o.growOnDemand
}

func (o *pooledWorkerPoolOptions) SetNumShards(value int64) PooledWorkerPoolOptions {
	opts := *o
	opts.numShards = value
	return &opts
}

func (o *pooledWorkerPoolOptions) NumShards() int64 {
	return o.numShards
}

func (o *pooledWorkerPoolOptions) SetKillWorkerProbability(value float64) PooledWorkerPoolOptions {
	opts := *o
	opts.killWorkerProbability = value
	return &opts
}

func (o *pooledWorkerPoolOptions) KillWorkerProbability() float64 {
	return o.killWorkerProbability
}

func (o *pooledWorkerPoolOptions) SetNowFn(value NowFn) PooledWorkerPoolOptions {
	opts := *o
	opts.nowFn = value
	return &opts
}

func (o *pooledWorkerPoolOptions) NowFn() NowFn {
	return o.nowFn
}

func (o *pooledWorkerPoolOptions) SetInstrumentOptions(value instrument.Options) PooledWorkerPoolOptions {
	opts := *o
	opts.iOpts = value
	return &opts
}

func (o *pooledWorkerPoolOptions) InstrumentOptions() instrument.Options {
	return o.iOpts
}
