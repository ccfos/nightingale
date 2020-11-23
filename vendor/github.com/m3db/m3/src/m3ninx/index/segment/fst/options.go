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

package fst

import (
	"github.com/m3db/m3/src/m3ninx/postings"
	"github.com/m3db/m3/src/m3ninx/postings/roaring"
	"github.com/m3db/m3/src/m3ninx/x/bytes"
	"github.com/m3db/m3/src/x/context"
	"github.com/m3db/m3/src/x/instrument"
	"github.com/m3db/m3/src/x/pool"
)

const (
	defaultBytesArrayPoolCapacity = 128
)

// Options is a collection of knobs for a fs segment.
type Options interface {
	// SetInstrumentOptions sets the instrument options.
	SetInstrumentOptions(value instrument.Options) Options

	// InstrumentOptions returns the instrument options.
	InstrumentOptions() instrument.Options

	// SetBytesPool sets the bytes pool.
	SetBytesPool(value pool.BytesPool) Options

	// BytesPool returns the bytes pool.
	BytesPool() pool.BytesPool

	// SetPostingsListPool sets the postings list pool.
	SetPostingsListPool(value postings.Pool) Options

	// PostingsListPool returns the postings list pool.
	PostingsListPool() postings.Pool

	// SetContextPool sets the contextPool.
	SetContextPool(value context.Pool) Options

	// ContextPool returns the contextPool.
	ContextPool() context.Pool
}

type opts struct {
	iopts             instrument.Options
	bytesSliceArrPool bytes.SliceArrayPool
	bytesPool         pool.BytesPool
	postingsPool      postings.Pool
	contextPool       context.Pool
}

// NewOptions returns new options.
func NewOptions() Options {
	arrPool := bytes.NewSliceArrayPool(bytes.SliceArrayPoolOpts{
		Capacity: defaultBytesArrayPoolCapacity,
		Options:  pool.NewObjectPoolOptions(),
	})
	arrPool.Init()

	bytesPool := pool.NewBytesPool([]pool.Bucket{
		{Capacity: 256, Count: 1024},
	}, nil)
	bytesPool.Init()

	return &opts{
		iopts:             instrument.NewOptions(),
		bytesSliceArrPool: arrPool,
		bytesPool:         bytesPool,
		postingsPool:      postings.NewPool(nil, roaring.NewPostingsList),
		// Use a zero pool, this should be overriden at config time.
		contextPool: context.NewPool(context.NewOptions().
			SetContextPoolOptions(pool.NewObjectPoolOptions().SetSize(0)).
			SetFinalizerPoolOptions(pool.NewObjectPoolOptions().SetSize(0))),
	}
}

func (o *opts) SetInstrumentOptions(v instrument.Options) Options {
	opts := *o
	opts.iopts = v
	return &opts
}

func (o *opts) InstrumentOptions() instrument.Options {
	return o.iopts
}

func (o *opts) SetBytesPool(value pool.BytesPool) Options {
	opts := *o
	opts.bytesPool = value
	return &opts
}

func (o *opts) BytesPool() pool.BytesPool {
	return o.bytesPool
}

func (o *opts) SetPostingsListPool(v postings.Pool) Options {
	opts := *o
	opts.postingsPool = v
	return &opts
}

func (o *opts) PostingsListPool() postings.Pool {
	return o.postingsPool
}

func (o *opts) SetContextPool(value context.Pool) Options {
	opts := *o
	opts.contextPool = value
	return &opts
}

func (o *opts) ContextPool() context.Pool {
	return o.contextPool
}
