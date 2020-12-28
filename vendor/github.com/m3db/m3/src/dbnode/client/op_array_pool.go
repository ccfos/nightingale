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

package client

import (
	"github.com/m3db/m3/src/x/pool"
)

type opArrayPool struct {
	pool     pool.ObjectPool
	capacity int
}

func newOpArrayPool(opts pool.ObjectPoolOptions, capacity int) *opArrayPool {
	p := pool.NewObjectPool(opts)
	return &opArrayPool{p, capacity}
}

func (p *opArrayPool) Init() {
	p.pool.Init(func() interface{} {
		return make([]op, 0, p.capacity)
	})
}

func (p *opArrayPool) Get() []op {
	return p.pool.Get().([]op)
}

func (p *opArrayPool) Put(ops []op) {
	for i := range ops {
		ops[i] = nil
	}
	ops = ops[:0]
	p.pool.Put(ops)
}

type fetchBatchOpArrayArrayPool struct {
	pool     pool.ObjectPool
	entries  int
	capacity int
}

func newFetchBatchOpArrayArrayPool(
	opts pool.ObjectPoolOptions,
	entries, capacity int,
) *fetchBatchOpArrayArrayPool {
	p := pool.NewObjectPool(opts)
	return &fetchBatchOpArrayArrayPool{
		pool:     p,
		entries:  entries,
		capacity: capacity,
	}
}

func (p *fetchBatchOpArrayArrayPool) Init() {
	p.pool.Init(func() interface{} {
		arr := make([][]*fetchBatchOp, p.entries)
		for i := range arr {
			arr[i] = make([]*fetchBatchOp, 0, p.capacity)
		}
		return arr
	})
}

func (p *fetchBatchOpArrayArrayPool) Get() [][]*fetchBatchOp {
	return p.pool.Get().([][]*fetchBatchOp)
}

func (p *fetchBatchOpArrayArrayPool) Put(arr [][]*fetchBatchOp) {
	for i := range arr {
		for j := range arr[i] {
			arr[i][j] = nil
		}
		arr[i] = arr[i][:0]
	}
	p.pool.Put(arr)
}

func (p *fetchBatchOpArrayArrayPool) Entries() int {
	return p.entries
}

func (p *fetchBatchOpArrayArrayPool) Capacity() int {
	return p.capacity
}
