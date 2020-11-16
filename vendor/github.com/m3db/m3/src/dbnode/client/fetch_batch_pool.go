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

var (
	fetchBatchRawV2RequestZero rpc.FetchBatchRawV2Request
)

type fetchBatchRawV2RequestPool interface {
	// Init pool.
	Init()

	// Get a write batch request.
	Get() *rpc.FetchBatchRawV2Request

	// Put a write batch request.
	Put(w *rpc.FetchBatchRawV2Request)
}

type poolOfFetchBatchRawV2Request struct {
	pool pool.ObjectPool
}

func newFetchBatchRawV2RequestPool(opts pool.ObjectPoolOptions) fetchBatchRawV2RequestPool {
	p := pool.NewObjectPool(opts)
	return &poolOfFetchBatchRawV2Request{p}
}

func (p *poolOfFetchBatchRawV2Request) Init() {
	p.pool.Init(func() interface{} {
		return &rpc.FetchBatchRawV2Request{}
	})
}

func (p *poolOfFetchBatchRawV2Request) Get() *rpc.FetchBatchRawV2Request {
	return p.pool.Get().(*rpc.FetchBatchRawV2Request)
}

func (p *poolOfFetchBatchRawV2Request) Put(w *rpc.FetchBatchRawV2Request) {
	*w = fetchBatchRawV2RequestZero
	p.pool.Put(w)
}
