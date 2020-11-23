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

// Package pool provides implementations for object pooling.
package pool

import (
	"github.com/m3db/m3/src/x/checked"
	"github.com/m3db/m3/src/x/instrument"
)

// Allocator allocates an object for a pool.
type Allocator func() interface{}

// ObjectPool provides a pool for objects.
type ObjectPool interface {
	// Init initializes the pool.
	Init(alloc Allocator)

	// Get provides an object from the pool.
	Get() interface{}

	// Put returns an object to the pool.
	Put(obj interface{})
}

// OnPoolAccessErrorFn is a function to call when a pool access error occurs,
// such as get or put before the pool is initialized.
type OnPoolAccessErrorFn func(err error)

// ObjectPoolOptions provides options for an object pool.
type ObjectPoolOptions interface {
	// SetSize sets the size of the object pool.
	SetSize(value int) ObjectPoolOptions

	// Size returns the size of the object pool.
	Size() int

	// SetRefillLowWatermark sets the refill low watermark value between [0, 1),
	// if zero then no refills occur.
	SetRefillLowWatermark(value float64) ObjectPoolOptions

	// RefillLowWatermark returns the refill low watermark value between [0, 1),
	// if zero then no refills occur.
	RefillLowWatermark() float64

	// SetRefillHighWatermark sets the refill high watermark value between [0, 1),
	// if less or equal to low watermark then no refills occur.
	SetRefillHighWatermark(value float64) ObjectPoolOptions

	// RefillLowWatermark returns the refill low watermark value between [0, 1),
	// if less or equal to low watermark then no refills occur.
	RefillHighWatermark() float64

	// SetInstrumentOptions sets the instrument options.
	SetInstrumentOptions(value instrument.Options) ObjectPoolOptions

	// InstrumentOptions returns the instrument options.
	InstrumentOptions() instrument.Options

	// SetOnPoolAccessErrorFn sets the on pool access error callback, by
	// default this is a panic.
	SetOnPoolAccessErrorFn(value OnPoolAccessErrorFn) ObjectPoolOptions

	// OnPoolAccessErrorFn returns the on pool access error callback, by
	// default this is a panic.
	OnPoolAccessErrorFn() OnPoolAccessErrorFn
}

// Bucket specifies a pool bucket.
type Bucket struct {
	// Capacity is the size of each element in the bucket.
	Capacity int

	// Count is the number of fixed elements in the bucket.
	Count int

	// Options is an optional override to specify options to use for a bucket,
	// specify nil to use the options specified to the bucketized pool
	// constructor for this bucket.
	Options ObjectPoolOptions
}

// BucketByCapacity is a sortable collection of pool buckets.
type BucketByCapacity []Bucket

func (x BucketByCapacity) Len() int {
	return len(x)
}

func (x BucketByCapacity) Swap(i, j int) {
	x[i], x[j] = x[j], x[i]
}

func (x BucketByCapacity) Less(i, j int) bool {
	return x[i].Capacity < x[j].Capacity
}

// BucketizedAllocator allocates an object for a bucket given its capacity.
type BucketizedAllocator func(capacity int) interface{}

// BucketizedObjectPool is a bucketized pool of objects.
type BucketizedObjectPool interface {
	// Init initializes the pool.
	Init(alloc BucketizedAllocator)

	// Get provides an object from the pool.
	Get(capacity int) interface{}

	// Put returns an object to the pool, given the object capacity.
	Put(obj interface{}, capacity int)
}

// BytesPool provides a pool for variable size buffers.
type BytesPool interface {
	// Init initializes the pool.
	Init()

	// Get provides a buffer from the pool.
	Get(capacity int) []byte

	// Put returns a buffer to the pool.
	Put(buffer []byte)
}

// CheckedBytesPool provides a checked pool for variable size buffers.
type CheckedBytesPool interface {
	// Init initializes the pool.
	Init()

	// Get provides a buffer from the pool, to return it to the pool simply
	// increment it immediately, continue to increment and decrement through
	// use and when decremented to zero and finalized it will return itself
	// to the pool. The pool uses the finalizer on the checked.Bytes so be sure
	// not to override it.
	Get(capacity int) checked.Bytes

	// BytesPool returns the underlying bytes pool used, useful if required
	// to retrieve when only the checked bytes pool is accessible.
	BytesPool() BytesPool
}

// FloatsPool provides a pool for variable-sized float64 slices.
type FloatsPool interface {
	// Init initializes the pool.
	Init()

	// Get provides an float64 slice from the pool.
	Get(capacity int) []float64

	// Put returns an float64 slice to the pool.
	Put(value []float64)
}
