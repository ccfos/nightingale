// Copyright (c) 2019 Uber Technologies, Inc.
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

type fetchBatchRawV2RequestElementArrayPool interface {
	// Init pool
	Init()

	// Get an array of WriteBatchV2RawRequestElement objects
	Get() []*rpc.FetchBatchRawV2RequestElement

	// Put an array of FetchBatchRawV2RequestElement objects
	Put(w []*rpc.FetchBatchRawV2RequestElement)
}

type poolOfFetchBatchRawV2RequestElementArray struct {
	pool     pool.ObjectPool
	capacity int
}

func newFetchBatchRawV2RequestElementArrayPool(
	opts pool.ObjectPoolOptions, capacity int) fetchBatchRawV2RequestElementArrayPool {

	p := pool.NewObjectPool(opts)
	return &poolOfFetchBatchRawV2RequestElementArray{p, capacity}
}

func (p *poolOfFetchBatchRawV2RequestElementArray) Init() {
	p.pool.Init(func() interface{} {
		return make([]*rpc.FetchBatchRawV2RequestElement, 0, p.capacity)
	})
}

func (p *poolOfFetchBatchRawV2RequestElementArray) Get() []*rpc.FetchBatchRawV2RequestElement {
	return p.pool.Get().([]*rpc.FetchBatchRawV2RequestElement)
}

func (p *poolOfFetchBatchRawV2RequestElementArray) Put(w []*rpc.FetchBatchRawV2RequestElement) {
	for i := range w {
		w[i] = nil
	}
	w = w[:0]
	p.pool.Put(w)
}
