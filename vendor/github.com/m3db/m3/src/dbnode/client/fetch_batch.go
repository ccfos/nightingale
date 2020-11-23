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
	"github.com/m3db/m3/src/x/checked"
	"github.com/m3db/m3/src/x/pool"
)

type fetchBatchOp struct {
	checked.RefCount
	request           rpc.FetchBatchRawRequest
	requestV2Elements []rpc.FetchBatchRawV2RequestElement
	completionFns     []completionFn
	finalizer         fetchBatchOpFinalizer
}

func (f *fetchBatchOp) reset() {
	f.IncWrites()
	f.request.RangeStart = 0
	f.request.RangeEnd = 0
	f.request.NameSpace = nil
	for i := range f.request.Ids {
		f.request.Ids[i] = nil
	}
	f.request.Ids = f.request.Ids[:0]
	for i := range f.completionFns {
		f.completionFns[i] = nil
	}
	f.completionFns = f.completionFns[:0]

	for i := range f.requestV2Elements {
		f.requestV2Elements[i].NameSpace = 0
		f.requestV2Elements[i].RangeStart = 0
		f.requestV2Elements[i].RangeEnd = 0
		f.requestV2Elements[i].ID = nil
		f.requestV2Elements[i].RangeTimeType = 0
	}
	f.requestV2Elements = f.requestV2Elements[:0]

	f.DecWrites()
}

func (f *fetchBatchOp) append(namespace, id []byte, completionFn completionFn) {
	f.IncWrites()
	f.request.NameSpace = namespace
	f.request.Ids = append(f.request.Ids, id)
	f.requestV2Elements = append(f.requestV2Elements, rpc.FetchBatchRawV2RequestElement{
		// NameSpace filled in by the host queue later.
		RangeStart:    f.request.RangeStart,
		RangeEnd:      f.request.RangeEnd,
		ID:            id,
		RangeTimeType: f.request.RangeTimeType,
	})
	f.completionFns = append(f.completionFns, completionFn)
	f.DecWrites()
}

func (f *fetchBatchOp) Size() int {
	f.IncReads()
	value := len(f.request.Ids)
	if value == 0 {
		value = len(f.requestV2Elements)
	}
	f.DecReads()
	return value
}

func (f *fetchBatchOp) CompletionFn() completionFn {
	return f.completeAll
}

func (f *fetchBatchOp) completeAll(result interface{}, err error) {
	for idx := range f.completionFns {
		f.completionFns[idx](result, err)
	}
}

func (f *fetchBatchOp) complete(idx int, result interface{}, err error) {
	f.IncReads()
	fn := f.completionFns[idx]
	f.DecReads()
	fn(result, err)
}

type fetchBatchOpFinalizer struct {
	ref  *fetchBatchOp
	pool *fetchBatchOpPool
}

func (f fetchBatchOpFinalizer) OnFinalize() {
	f.pool.Put(f.ref)
}

type fetchBatchOpPool struct {
	pool     pool.ObjectPool
	capacity int
}

func newFetchBatchOpPool(opts pool.ObjectPoolOptions, capacity int) *fetchBatchOpPool {
	p := pool.NewObjectPool(opts)
	return &fetchBatchOpPool{pool: p, capacity: capacity}
}

func (p *fetchBatchOpPool) Init() {
	p.pool.Init(func() interface{} {
		f := &fetchBatchOp{}
		f.request.Ids = make([][]byte, 0, p.capacity)
		f.completionFns = make([]completionFn, 0, p.capacity)
		f.finalizer.ref = f
		f.finalizer.pool = p
		f.SetOnFinalize(&f.finalizer)

		f.IncRef()
		f.reset()
		f.DecRef()
		return f
	})
}

func (p *fetchBatchOpPool) Get() *fetchBatchOp {
	return p.pool.Get().(*fetchBatchOp)
}

func (p *fetchBatchOpPool) Put(f *fetchBatchOp) {
	f.IncRef()
	if cap(f.request.Ids) != p.capacity || cap(f.completionFns) != p.capacity {
		// Grew outside capacity, do not return to pool
		f.DecRef()
		return
	}
	f.reset()
	f.DecRef()
	p.pool.Put(f)
}
