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
	"github.com/m3db/m3/src/dbnode/generated/thrift/rpc"
	"github.com/m3db/m3/src/x/pool"
)

type writeBatchRawRequestElementArrayPool interface {
	// Init pool
	Init()

	// Get an array of WriteBatchRawRequestElement objects
	Get() []*rpc.WriteBatchRawRequestElement

	// Put an array of WriteBatchRawRequestElement objects
	Put(w []*rpc.WriteBatchRawRequestElement)
}

type poolOfWriteBatchRawRequestElementArray struct {
	pool     pool.ObjectPool
	capacity int
}

func newWriteBatchRawRequestElementArrayPool(
	opts pool.ObjectPoolOptions, capacity int) writeBatchRawRequestElementArrayPool {

	p := pool.NewObjectPool(opts)
	return &poolOfWriteBatchRawRequestElementArray{p, capacity}
}

func (p *poolOfWriteBatchRawRequestElementArray) Init() {
	p.pool.Init(func() interface{} {
		return make([]*rpc.WriteBatchRawRequestElement, 0, p.capacity)
	})
}

func (p *poolOfWriteBatchRawRequestElementArray) Get() []*rpc.WriteBatchRawRequestElement {
	return p.pool.Get().([]*rpc.WriteBatchRawRequestElement)
}

func (p *poolOfWriteBatchRawRequestElementArray) Put(w []*rpc.WriteBatchRawRequestElement) {
	for i := range w {
		w[i] = nil
	}
	w = w[:0]
	p.pool.Put(w)
}

type writeBatchRawV2RequestElementArrayPool interface {
	// Init pool
	Init()

	// Get an array of WriteBatchV2RawRequestElement objects
	Get() []*rpc.WriteBatchRawV2RequestElement

	// Put an array of WriteBatchRawV2RequestElement objects
	Put(w []*rpc.WriteBatchRawV2RequestElement)
}

type poolOfWriteBatchRawV2RequestElementArray struct {
	pool     pool.ObjectPool
	capacity int
}

func newWriteBatchRawV2RequestElementArrayPool(
	opts pool.ObjectPoolOptions, capacity int) writeBatchRawV2RequestElementArrayPool {

	p := pool.NewObjectPool(opts)
	return &poolOfWriteBatchRawV2RequestElementArray{p, capacity}
}

func (p *poolOfWriteBatchRawV2RequestElementArray) Init() {
	p.pool.Init(func() interface{} {
		return make([]*rpc.WriteBatchRawV2RequestElement, 0, p.capacity)
	})
}

func (p *poolOfWriteBatchRawV2RequestElementArray) Get() []*rpc.WriteBatchRawV2RequestElement {
	return p.pool.Get().([]*rpc.WriteBatchRawV2RequestElement)
}

func (p *poolOfWriteBatchRawV2RequestElementArray) Put(w []*rpc.WriteBatchRawV2RequestElement) {
	for i := range w {
		w[i] = nil
	}
	w = w[:0]
	p.pool.Put(w)
}
