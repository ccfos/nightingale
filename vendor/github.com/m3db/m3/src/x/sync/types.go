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
	"time"

	"github.com/m3db/m3/src/x/instrument"
)

// Work is a unit of item to be worked on.
type Work func()

// PooledWorkerPool provides a pool for goroutines, but unlike WorkerPool,
// the actual goroutines themselves are re-used. This can be useful from a
// performance perspective in scenarios where the allocation and growth of
// the new goroutine and its stack is a bottleneck. Specifically, if the
// work function being performed has a very deep call-stack, calls to
// runtime.morestack can dominate the workload. Re-using existing goroutines
// allows the stack to be grown once, and then re-used for many invocations.
//
// In order to prevent abnormally large goroutine stacks from persisting over
// the life-cycle of an application, the PooledWorkerPool will randomly kill
// existing goroutines and spawn a new one.
//
// The PooledWorkerPool also implements sharding of its underlying worker channels
// to prevent excessive lock contention.
type PooledWorkerPool interface {
	// Init initializes the pool.
	Init()

	// Go assign the Work to be executed by a Goroutine. Whether or not
	// it waits for an existing Goroutine to become available or not
	// is determined by the GrowOnDemand() option. If GrowOnDemand is not
	// set then the call to Go() will block until a goroutine is available.
	// If GrowOnDemand() is set then it will expand the pool of goroutines to
	// accommodate the work. The newly allocated goroutine will temporarily
	// participate in the pool in an effort to amortize its allocation cost, but
	// will eventually be killed. This allows the pool to dynamically respond to
	// workloads without causing excessive memory pressure. The pool will grow in
	// size when the workload exceeds its capacity and shrink back down to its
	// original size if/when the burst subsides.
	Go(work Work)

	// GoWithTimeout waits up to the given timeout for a worker to become
	// available, returning true if a worker becomes available, or false
	// otherwise.
	GoWithTimeout(work Work, timeout time.Duration) bool
}

// WorkerPool provides a pool for goroutines.
type WorkerPool interface {
	// Init initializes the pool.
	Init()

	// Go waits until the next wbyorker becomes available and executes it.
	Go(work Work)

	// GoIfAvailable performs the work inside a worker if one is available and
	// returns true, or false otherwise.
	GoIfAvailable(work Work) bool

	// GoWithTimeout waits up to the given timeout for a worker to become
	// available, returning true if a worker becomes available, or false
	// otherwise.
	GoWithTimeout(work Work, timeout time.Duration) bool
}

// PooledWorkerPoolOptions is the options for a PooledWorkerPool.
type PooledWorkerPoolOptions interface {
	// SetGrowOnDemand sets whether the GrowOnDemand feature is enabled.
	SetGrowOnDemand(value bool) PooledWorkerPoolOptions

	// GrowOnDemand returns whether the GrowOnDemand feature is enabled.
	GrowOnDemand() bool

	// SetNumShards sets the number of worker channel shards.
	SetNumShards(value int64) PooledWorkerPoolOptions

	// NumShards returns the number of worker channel shards.
	NumShards() int64

	// SetKillWorkerProbability sets the probability to kill a worker.
	SetKillWorkerProbability(value float64) PooledWorkerPoolOptions

	// KillWorkerProbability returns the probability to kill a worker.
	KillWorkerProbability() float64

	// SetNowFn sets the now function.
	SetNowFn(value NowFn) PooledWorkerPoolOptions

	// NowFn returns the now function.
	NowFn() NowFn

	// SetInstrumentOptions sets the instrument options.
	SetInstrumentOptions(value instrument.Options) PooledWorkerPoolOptions

	// InstrumentOptions returns the now function.
	InstrumentOptions() instrument.Options
}
