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
	aggregateOpRequestZeroed = rpc.AggregateQueryRawRequest{}
)

type aggregateOp struct {
	refCounter
	request      rpc.AggregateQueryRawRequest
	completionFn completionFn

	pool aggregateOpPool
}

func (f *aggregateOp) Size() int                  { return 1 }
func (f *aggregateOp) CompletionFn() completionFn { return f.completionFn }

func (f *aggregateOp) update(req rpc.AggregateQueryRawRequest, fn completionFn) {
	f.request = req
	f.completionFn = fn
}

func (f *aggregateOp) requestLimit(defaultValue int) int {
	if f.request.Limit == nil {
		return defaultValue
	}
	return int(*f.request.Limit)
}

func (f *aggregateOp) close() {
	f.completionFn = nil
	f.request = aggregateOpRequestZeroed
	// return to pool
	if f.pool == nil {
		return
	}
	f.pool.Put(f)
}

func newAggregateOp(p aggregateOpPool) *aggregateOp {
	f := &aggregateOp{pool: p}
	f.destructorFn = f.close
	return f
}

type aggregateOpPool interface {
	Init()
	Get() *aggregateOp
	Put(*aggregateOp)
}

type aggregateOpPoolImpl struct {
	pool pool.ObjectPool
}

func newAggregateOpPool(
	opts pool.ObjectPoolOptions,
) aggregateOpPool {
	p := pool.NewObjectPool(opts)
	return &aggregateOpPoolImpl{pool: p}
}

func (p *aggregateOpPoolImpl) Init() {
	p.pool.Init(func() interface{} {
		return newAggregateOp(p)
	})
}

func (p *aggregateOpPoolImpl) Get() *aggregateOp {
	return p.pool.Get().(*aggregateOp)
}

func (p *aggregateOpPoolImpl) Put(f *aggregateOp) {
	p.pool.Put(f)
}
