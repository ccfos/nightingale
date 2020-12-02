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
	fetchTaggedOpRequestZeroed = rpc.FetchTaggedRequest{}
)

type fetchTaggedOp struct {
	refCounter
	request      rpc.FetchTaggedRequest
	completionFn completionFn

	pool fetchTaggedOpPool
}

func (f *fetchTaggedOp) Size() int                  { return 1 }
func (f *fetchTaggedOp) CompletionFn() completionFn { return f.completionFn }

func (f *fetchTaggedOp) update(req rpc.FetchTaggedRequest, fn completionFn) {
	f.request = req
	f.completionFn = fn
}

func (f *fetchTaggedOp) requestLimit(defaultValue int) int {
	if f.request.Limit == nil {
		return defaultValue
	}
	return int(*f.request.Limit)
}

func (f *fetchTaggedOp) close() {
	f.completionFn = nil
	f.request = fetchTaggedOpRequestZeroed
	// return to pool
	if f.pool == nil {
		return
	}
	f.pool.Put(f)
}

func newFetchTaggedOp(p fetchTaggedOpPool) *fetchTaggedOp {
	f := &fetchTaggedOp{pool: p}
	f.destructorFn = f.close
	return f
}

type fetchTaggedOpPool interface {
	Init()
	Get() *fetchTaggedOp
	Put(*fetchTaggedOp)
}

type fetchTaggedOpPoolImpl struct {
	pool pool.ObjectPool
}

func newFetchTaggedOpPool(
	opts pool.ObjectPoolOptions,
) fetchTaggedOpPool {
	p := pool.NewObjectPool(opts)
	return &fetchTaggedOpPoolImpl{pool: p}
}

func (p *fetchTaggedOpPoolImpl) Init() {
	p.pool.Init(func() interface{} {
		return newFetchTaggedOp(p)
	})
}

func (p *fetchTaggedOpPoolImpl) Get() *fetchTaggedOp {
	return p.pool.Get().(*fetchTaggedOp)
}

func (p *fetchTaggedOpPoolImpl) Put(f *fetchTaggedOp) {
	p.pool.Put(f)
}
