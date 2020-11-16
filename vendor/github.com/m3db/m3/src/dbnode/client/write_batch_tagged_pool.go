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

var (
	writeTaggedBatchRawRequestZeroed   rpc.WriteTaggedBatchRawRequest
	writeTaggedBatchRawV2RequestZeroed rpc.WriteTaggedBatchRawV2Request
)

type writeTaggedBatchRawRequestPool interface {
	// Init pool
	Init()

	// Get a write batch request
	Get() *rpc.WriteTaggedBatchRawRequest

	// Put a write batch request
	Put(w *rpc.WriteTaggedBatchRawRequest)
}

type poolOfWriteTaggedBatchRawRequest struct {
	pool pool.ObjectPool
}

func newWriteTaggedBatchRawRequestPool(opts pool.ObjectPoolOptions) writeTaggedBatchRawRequestPool {
	p := pool.NewObjectPool(opts)
	return &poolOfWriteTaggedBatchRawRequest{p}
}

func (p *poolOfWriteTaggedBatchRawRequest) Init() {
	p.pool.Init(func() interface{} {
		return &rpc.WriteTaggedBatchRawRequest{}
	})
}

func (p *poolOfWriteTaggedBatchRawRequest) Get() *rpc.WriteTaggedBatchRawRequest {
	return p.pool.Get().(*rpc.WriteTaggedBatchRawRequest)
}

func (p *poolOfWriteTaggedBatchRawRequest) Put(w *rpc.WriteTaggedBatchRawRequest) {
	*w = writeTaggedBatchRawRequestZeroed
	p.pool.Put(w)
}

type writeTaggedBatchRawV2RequestPool interface {
	// Init pool.
	Init()

	// Get a write batch request.
	Get() *rpc.WriteTaggedBatchRawV2Request

	// Put a write batch request.
	Put(w *rpc.WriteTaggedBatchRawV2Request)
}

type poolOfWriteTaggedBatchRawV2Request struct {
	pool pool.ObjectPool
}

func newWriteTaggedBatchRawV2RequestPool(opts pool.ObjectPoolOptions) writeTaggedBatchRawV2RequestPool {
	p := pool.NewObjectPool(opts)
	return &poolOfWriteTaggedBatchRawV2Request{p}
}

func (p *poolOfWriteTaggedBatchRawV2Request) Init() {
	p.pool.Init(func() interface{} {
		return &rpc.WriteTaggedBatchRawV2Request{}
	})
}

func (p *poolOfWriteTaggedBatchRawV2Request) Get() *rpc.WriteTaggedBatchRawV2Request {
	return p.pool.Get().(*rpc.WriteTaggedBatchRawV2Request)
}

func (p *poolOfWriteTaggedBatchRawV2Request) Put(w *rpc.WriteTaggedBatchRawV2Request) {
	*w = writeTaggedBatchRawV2RequestZeroed
	p.pool.Put(w)
}
