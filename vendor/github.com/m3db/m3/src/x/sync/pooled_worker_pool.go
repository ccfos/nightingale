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
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MichaelTJones/pcg"
	"github.com/uber-go/tally"
)

const (
	numGoroutinesGaugeSampleRate = 1000
)

var (
	pooledWorkerPoolGoroutinesCapacity *int
)

type pooledWorkerPool struct {
	sync.Mutex
	numRoutinesAtomic        int64
	numWorkingRoutinesAtomic int64
	numRoutinesGauge         tally.Gauge
	numWorkingRoutinesGauge  tally.Gauge
	growOnDemand             bool
	workChs                  []chan Work
	numShards                int64
	killWorkerProbability    float64
	nowFn                    NowFn
}

// NewPooledWorkerPool creates a new worker pool.
func NewPooledWorkerPool(size int, opts PooledWorkerPoolOptions) (PooledWorkerPool, error) {
	if size <= 0 {
		return nil, fmt.Errorf("pooled worker pool size too small: %d", size)
	}

	numShards := opts.NumShards()
	if int64(size) < numShards {
		numShards = int64(size)
	}

	workChs := make([]chan Work, numShards)
	for i := range workChs {
		if c := pooledWorkerPoolGoroutinesCapacity; c != nil {
			if *c == 0 {
				workChs[i] = make(chan Work)
			} else {
				workChs[i] = make(chan Work, *c)
			}
		} else {
			workChs[i] = make(chan Work, int64(size)/numShards)
		}
	}

	return &pooledWorkerPool{
		numRoutinesAtomic:        0,
		numWorkingRoutinesAtomic: 0,
		numRoutinesGauge:         opts.InstrumentOptions().MetricsScope().Gauge("num-routines"),
		numWorkingRoutinesGauge:  opts.InstrumentOptions().MetricsScope().Gauge("num-working-routines"),
		growOnDemand:             opts.GrowOnDemand(),
		workChs:                  workChs,
		numShards:                numShards,
		killWorkerProbability:    opts.KillWorkerProbability(),
		nowFn:                    opts.NowFn(),
	}, nil
}

func (p *pooledWorkerPool) Init() {
	rng := pcg.NewPCG64() // Just use default seed here
	for _, workCh := range p.workChs {
		for i := 0; i < cap(workCh); i++ {
			p.spawnWorker(rng.Random(), nil, workCh, true)
		}
	}
}

func (p *pooledWorkerPool) Go(work Work) {
	p.goWithTimeout(work, 0)
}

func (p *pooledWorkerPool) GoWithTimeout(work Work, timeout time.Duration) bool {
	return p.goWithTimeout(work, timeout)
}

func (p *pooledWorkerPool) goWithTimeout(work Work, timeout time.Duration) bool {
	var (
		// Use time.Now() to avoid excessive synchronization
		currTime  = p.nowFn().UnixNano()
		workChIdx = currTime % p.numShards
		workCh    = p.workChs[workChIdx]
	)

	if currTime%numGoroutinesGaugeSampleRate == 0 {
		p.emitNumRoutines()
		p.emitNumWorkingRoutines()
	}

	if !p.growOnDemand {
		if timeout <= 0 {
			workCh <- work
			return true
		}

		// Attempt to try writing without allocating a ticker.
		select {
		case workCh <- work:
			return true
		default:
		}

		// Now allocate a ticker and attempt a write.
		ticker := time.NewTicker(timeout)
		defer ticker.Stop()

		select {
		case workCh <- work:
			return true
		case <-ticker.C:
			return false
		}
	}

	select {
	case workCh <- work:
	default:
		// If the queue for the worker we were assigned to is full,
		// allocate a new goroutine to do the work and then
		// assign it to be a temporary additional worker for the queue.
		// This allows the worker pool to accommodate "bursts" of
		// traffic. Also, it reduces the need for operators to tune the size
		// of the pool for a given workload. If the pool is initially
		// sized too small, it will eventually grow to accommodate the
		// workload, and if the workload decreases the killWorkerProbability
		// will slowly shrink the pool back down to its original size because
		// workers created in this manner will not spawn their replacement
		// before killing themselves.
		p.spawnWorker(uint64(currTime), work, workCh, false)
	}
	return true
}

func (p *pooledWorkerPool) spawnWorker(
	seed uint64, initialWork Work, workCh chan Work, spawnReplacement bool) {
	go func() {
		p.incNumRoutines()
		if initialWork != nil {
			initialWork()
		}

		// RNG per worker to avoid synchronization.
		var (
			rng = pcg.NewPCG64().Seed(seed, seed*2, seed*3, seed*4)
			// killWorkerProbability is a float but but the PCG RNG only
			// generates uint64s so we need to identify the uint64 number
			// that corresponds to the equivalent probability assuming we're
			// generating random numbers in the entire uint64 range. For example,
			// if the max uint64 was 1000 and we had a killWorkerProbability of 0.15
			// then the killThreshold should be 0.15 * 1000 = 150 if we want a randomly
			// chosen number between 0 and 1000 to have a 15% chance of being below
			// the selected threshold.
			killThreshold = uint64(p.killWorkerProbability * float64(math.MaxUint64))
		)
		for f := range workCh {
			p.incNumWorkingRoutines()
			f()
			p.decNumWorkingRoutines()
			if rng.Random() < killThreshold {
				if spawnReplacement {
					p.spawnWorker(rng.Random(), nil, workCh, true)
				}
				p.decNumRoutines()
				return
			}
		}
	}()
}

func (p *pooledWorkerPool) emitNumRoutines() {
	numRoutines := float64(p.getNumRoutines())
	p.numRoutinesGauge.Update(numRoutines)
}

func (p *pooledWorkerPool) incNumRoutines() {
	atomic.AddInt64(&p.numRoutinesAtomic, 1)
}

func (p *pooledWorkerPool) decNumRoutines() {
	atomic.AddInt64(&p.numRoutinesAtomic, -1)
}

func (p *pooledWorkerPool) getNumRoutines() int64 {
	return atomic.LoadInt64(&p.numRoutinesAtomic)
}

func (p *pooledWorkerPool) emitNumWorkingRoutines() {
	numRoutines := float64(p.getNumWorkingRoutines())
	p.numWorkingRoutinesGauge.Update(numRoutines)
}

func (p *pooledWorkerPool) incNumWorkingRoutines() {
	atomic.AddInt64(&p.numWorkingRoutinesAtomic, 1)
}

func (p *pooledWorkerPool) decNumWorkingRoutines() {
	atomic.AddInt64(&p.numWorkingRoutinesAtomic, -1)
}

func (p *pooledWorkerPool) getNumWorkingRoutines() int64 {
	return atomic.LoadInt64(&p.numWorkingRoutinesAtomic)
}
