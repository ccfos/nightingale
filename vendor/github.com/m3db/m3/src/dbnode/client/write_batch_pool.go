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

var (
	writeBatchRawRequestZeroed   rpc.WriteBatchRawRequest
	writeBatchRawV2RequestZeroed rpc.WriteBatchRawV2Request
)

type writeBatchRawRequestPool interface {
	// Init pool
	Init()

	// Get a write batch request
	Get() *rpc.WriteBatchRawRequest

	// Put a write batch request
	Put(w *rpc.WriteBatchRawRequest)
}

type poolOfWriteBatchRawRequest struct {
	pool pool.ObjectPool
}

func newWriteBatchRawRequestPool(opts pool.ObjectPoolOptions) writeBatchRawRequestPool {
	p := pool.NewObjectPool(opts)
	return &poolOfWriteBatchRawRequest{p}
}

func (p *poolOfWriteBatchRawRequest) Init() {
	p.pool.Init(func() interface{} {
		return &rpc.WriteBatchRawRequest{}
	})
}

func (p *poolOfWriteBatchRawRequest) Get() *rpc.WriteBatchRawRequest {
	return p.pool.Get().(*rpc.WriteBatchRawRequest)
}

func (p *poolOfWriteBatchRawRequest) Put(w *rpc.WriteBatchRawRequest) {
	*w = writeBatchRawRequestZeroed
	p.pool.Put(w)
}

type writeBatchRawV2RequestPool interface {
	// Init pool.
	Init()

	// Get a write batch request.
	Get() *rpc.WriteBatchRawV2Request

	// Put a write batch request.
	Put(w *rpc.WriteBatchRawV2Request)
}

type poolOfWriteBatchRawV2Request struct {
	pool pool.ObjectPool
}

func newWriteBatchRawV2RequestPool(opts pool.ObjectPoolOptions) writeBatchRawV2RequestPool {
	p := pool.NewObjectPool(opts)
	return &poolOfWriteBatchRawV2Request{p}
}

func (p *poolOfWriteBatchRawV2Request) Init() {
	p.pool.Init(func() interface{} {
		return &rpc.WriteBatchRawV2Request{}
	})
}

func (p *poolOfWriteBatchRawV2Request) Get() *rpc.WriteBatchRawV2Request {
	return p.pool.Get().(*rpc.WriteBatchRawV2Request)
}

func (p *poolOfWriteBatchRawV2Request) Put(w *rpc.WriteBatchRawV2Request) {
	*w = writeBatchRawV2RequestZeroed
	p.pool.Put(w)
}
