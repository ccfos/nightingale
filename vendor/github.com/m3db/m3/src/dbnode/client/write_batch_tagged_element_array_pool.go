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

package client

import (
	"github.com/m3db/m3/src/dbnode/generated/thrift/rpc"
	"github.com/m3db/m3/src/x/pool"
)

type writeTaggedBatchRawRequestElementArrayPool interface {
	// Init pool
	Init()

	// Get an array of WriteTaggedBatchRawRequestElement objects
	Get() []*rpc.WriteTaggedBatchRawRequestElement

	// Put an array of WriteTaggedBatchRawRequestElement objects
	Put(w []*rpc.WriteTaggedBatchRawRequestElement)
}

type poolOfWriteTaggedBatchRawRequestElementArray struct {
	pool     pool.ObjectPool
	capacity int
}

func newWriteTaggedBatchRawRequestElementArrayPool(
	opts pool.ObjectPoolOptions, capacity int) writeTaggedBatchRawRequestElementArrayPool {

	p := pool.NewObjectPool(opts)
	return &poolOfWriteTaggedBatchRawRequestElementArray{p, capacity}
}

func (p *poolOfWriteTaggedBatchRawRequestElementArray) Init() {
	p.pool.Init(func() interface{} {
		return make([]*rpc.WriteTaggedBatchRawRequestElement, 0, p.capacity)
	})
}

func (p *poolOfWriteTaggedBatchRawRequestElementArray) Get() []*rpc.WriteTaggedBatchRawRequestElement {
	return p.pool.Get().([]*rpc.WriteTaggedBatchRawRequestElement)
}

func (p *poolOfWriteTaggedBatchRawRequestElementArray) Put(w []*rpc.WriteTaggedBatchRawRequestElement) {
	for i := range w {
		w[i] = nil
	}
	w = w[:0]
	p.pool.Put(w)
}

type writeTaggedBatchRawV2RequestElementArrayPool interface {
	// Init pool
	Init()

	// Get an array of WriteTaggedBatchRawV2RequestElement objects
	Get() []*rpc.WriteTaggedBatchRawV2RequestElement

	// Put an array of WriteTaggedBatchRawV2RequestElement objects
	Put(w []*rpc.WriteTaggedBatchRawV2RequestElement)
}

type poolOfWriteTaggedBatchRawV2RequestElementArray struct {
	pool     pool.ObjectPool
	capacity int
}

func newWriteTaggedBatchRawV2RequestElementArrayPool(
	opts pool.ObjectPoolOptions, capacity int) writeTaggedBatchRawV2RequestElementArrayPool {

	p := pool.NewObjectPool(opts)
	return &poolOfWriteTaggedBatchRawV2RequestElementArray{p, capacity}
}

func (p *poolOfWriteTaggedBatchRawV2RequestElementArray) Init() {
	p.pool.Init(func() interface{} {
		return make([]*rpc.WriteTaggedBatchRawV2RequestElement, 0, p.capacity)
	})
}

func (p *poolOfWriteTaggedBatchRawV2RequestElementArray) Get() []*rpc.WriteTaggedBatchRawV2RequestElement {
	return p.pool.Get().([]*rpc.WriteTaggedBatchRawV2RequestElement)
}

func (p *poolOfWriteTaggedBatchRawV2RequestElementArray) Put(w []*rpc.WriteTaggedBatchRawV2RequestElement) {
	for i := range w {
		w[i] = nil
	}
	w = w[:0]
	p.pool.Put(w)
}
