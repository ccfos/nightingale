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

package pool

import (
	"errors"
	"math"
	"sync/atomic"

	"github.com/m3db/m3/src/x/unsafe"

	"github.com/uber-go/tally"
	"golang.org/x/sys/cpu"
)

var (
	errPoolAlreadyInitialized      = errors.New("object pool already initialized")
	errPoolAccessBeforeInitialized = errors.New("object pool accessed before it was initialized")
)

const sampleObjectPoolLengthEvery = 2048

type objectPool struct {
	_                   cpu.CacheLinePad
	values              chan interface{}
	_                   cpu.CacheLinePad
	alloc               Allocator
	metrics             objectPoolMetrics
	onPoolAccessErrorFn OnPoolAccessErrorFn
	size                int
	refillLowWatermark  int
	refillHighWatermark int
	filling             int32
	initialized         int32
}

type objectPoolMetrics struct {
	free       tally.Gauge
	total      tally.Gauge
	getOnEmpty tally.Counter
	putOnFull  tally.Counter
}

// NewObjectPool creates a new pool
func NewObjectPool(opts ObjectPoolOptions) ObjectPool {
	if opts == nil {
		opts = NewObjectPoolOptions()
	}

	m := opts.InstrumentOptions().MetricsScope()

	p := &objectPool{
		size: opts.Size(),
		refillLowWatermark: int(math.Ceil(
			opts.RefillLowWatermark() * float64(opts.Size()))),
		refillHighWatermark: int(math.Ceil(
			opts.RefillHighWatermark() * float64(opts.Size()))),
		metrics: objectPoolMetrics{
			free:       m.Gauge("free"),
			total:      m.Gauge("total"),
			getOnEmpty: m.Counter("get-on-empty"),
			putOnFull:  m.Counter("put-on-full"),
		},
		onPoolAccessErrorFn: opts.OnPoolAccessErrorFn(),
		alloc: func() interface{} {
			fn := opts.OnPoolAccessErrorFn()
			fn(errPoolAccessBeforeInitialized)
			return nil
		},
	}

	p.setGauges()

	return p
}

func (p *objectPool) Init(alloc Allocator) {
	if !atomic.CompareAndSwapInt32(&p.initialized, 0, 1) {
		p.onPoolAccessErrorFn(errPoolAlreadyInitialized)
		return
	}

	p.values = make(chan interface{}, p.size)
	for i := 0; i < cap(p.values); i++ {
		p.values <- alloc()
	}

	p.alloc = alloc
	p.setGauges()
}

func (p *objectPool) Get() interface{} {
	var (
		metrics = p.metrics
		v       interface{}
	)

	select {
	case v = <-p.values:
	default:
		v = p.alloc()
		metrics.getOnEmpty.Inc(1)
	}

	if unsafe.Fastrandn(sampleObjectPoolLengthEvery) == 0 {
		// inlined setGauges()
		metrics.free.Update(float64(len(p.values)))
		metrics.total.Update(float64(p.size))
	}

	if p.refillLowWatermark > 0 && len(p.values) <= p.refillLowWatermark {
		p.tryFill()
	}

	return v
}

func (p *objectPool) Put(obj interface{}) {
	if p.values == nil {
		p.onPoolAccessErrorFn(errPoolAccessBeforeInitialized)
		return
	}
	select {
	case p.values <- obj:
	default:
		p.metrics.putOnFull.Inc(1)
	}
}

func (p *objectPool) setGauges() {
	p.metrics.free.Update(float64(len(p.values)))
	p.metrics.total.Update(float64(p.size))
}

func (p *objectPool) tryFill() {
	if !atomic.CompareAndSwapInt32(&p.filling, 0, 1) {
		return
	}

	go func() {
		defer atomic.StoreInt32(&p.filling, 0)

		for len(p.values) < p.refillHighWatermark {
			select {
			case p.values <- p.alloc():
			default:
				return
			}
		}
	}()
}
