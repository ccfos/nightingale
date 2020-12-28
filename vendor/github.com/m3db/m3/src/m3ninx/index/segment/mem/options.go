// Copyright (c) 2017 Uber Technologies, Inc.
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

package mem

import (
	"github.com/m3db/m3/src/m3ninx/postings"
	"github.com/m3db/m3/src/m3ninx/postings/roaring"
	"github.com/m3db/m3/src/m3ninx/util"
	"github.com/m3db/m3/src/m3ninx/x/bytes"
	"github.com/m3db/m3/src/x/instrument"
	"github.com/m3db/m3/src/x/pool"
)

const (
	defaultInitialCapacity        = 1024
	defaultBytesArrayPoolCapacity = 1024
	// This pool is used in a single-threaded manner.
	defaultBytesArrayPoolSize = 1
	// 2<<23 * 16 bytes (byte slice pointer) * 2 (Golang G.C) ~=
	// 0.5 GiB max memory usage.
	defaultBytesArrayPoolMaxArrayCapacity = 2 << 23
)

// Options is a collection of knobs for an in-memory segment.
type Options interface {
	// SetInstrumentOptions sets the instrument options.
	SetInstrumentOptions(value instrument.Options) Options

	// InstrumentOptions returns the instrument options.
	InstrumentOptions() instrument.Options

	// SetBytesSliceArrayPool sets the bytes slice array pool.
	SetBytesSliceArrayPool(value bytes.SliceArrayPool) Options

	// BytesSliceArrayPool returns the bytes slice array pool.
	BytesSliceArrayPool() bytes.SliceArrayPool

	// SetPostingsListPool sets the postings list pool.
	SetPostingsListPool(value postings.Pool) Options

	// PostingsListPool returns the postings list pool.
	PostingsListPool() postings.Pool

	// SetInitialCapacity sets the initial capacity.
	SetInitialCapacity(value int) Options

	// InitialCapacity returns the initial capacity.
	InitialCapacity() int

	// SetNewUUIDFn sets the function used to generate new UUIDs.
	SetNewUUIDFn(value util.NewUUIDFn) Options

	// NewUUIDFn returns the function used to generate new UUIDs.
	NewUUIDFn() util.NewUUIDFn
}

type opts struct {
	iopts             instrument.Options
	bytesSliceArrPool bytes.SliceArrayPool
	postingsPool      postings.Pool
	initialCapacity   int
	newUUIDFn         util.NewUUIDFn
}

// NewOptions returns new options.
func NewOptions() Options {
	arrPool := bytes.NewSliceArrayPool(bytes.SliceArrayPoolOpts{
		Capacity:    defaultBytesArrayPoolCapacity,
		MaxCapacity: defaultBytesArrayPoolMaxArrayCapacity,
		Options: pool.NewObjectPoolOptions().
			SetSize(defaultBytesArrayPoolSize).
			SetRefillLowWatermark(0).
			SetRefillHighWatermark(0),
	})
	arrPool.Init()
	return &opts{
		iopts:             instrument.NewOptions(),
		bytesSliceArrPool: arrPool,
		postingsPool:      postings.NewPool(nil, roaring.NewPostingsList),
		initialCapacity:   defaultInitialCapacity,
		newUUIDFn:         util.NewUUID,
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

func (o *opts) SetBytesSliceArrayPool(value bytes.SliceArrayPool) Options {
	opts := *o
	opts.bytesSliceArrPool = value
	return &opts
}

func (o *opts) BytesSliceArrayPool() bytes.SliceArrayPool {
	return o.bytesSliceArrPool
}

func (o *opts) SetPostingsListPool(v postings.Pool) Options {
	opts := *o
	opts.postingsPool = v
	return &opts
}

func (o *opts) PostingsListPool() postings.Pool {
	return o.postingsPool
}

func (o *opts) SetInitialCapacity(v int) Options {
	opts := *o
	opts.initialCapacity = v
	return &opts
}

func (o *opts) InitialCapacity() int {
	return o.initialCapacity
}

func (o *opts) SetNewUUIDFn(v util.NewUUIDFn) Options {
	opts := *o
	opts.newUUIDFn = v
	return &opts
}

func (o *opts) NewUUIDFn() util.NewUUIDFn {
	return o.newUUIDFn
}
