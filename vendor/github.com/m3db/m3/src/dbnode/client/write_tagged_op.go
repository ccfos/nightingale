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
	"math"

	"github.com/m3db/m3/src/dbnode/generated/thrift/rpc"
	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/m3/src/x/pool"
)

var (
	// NB(bl): use an invalid shardID for the zerod op
	writeTaggedOperationZeroed = writeTaggedOperation{shardID: math.MaxUint32}
)

type writeTaggedOperation struct {
	namespace    ident.ID
	shardID      uint32
	request      rpc.WriteTaggedBatchRawRequestElement
	requestV2    rpc.WriteTaggedBatchRawV2RequestElement
	datapoint    rpc.Datapoint
	completionFn completionFn
	pool         *writeTaggedOperationPool
}

func (w *writeTaggedOperation) reset() {
	*w = writeTaggedOperationZeroed
	w.request.Datapoint = &w.datapoint
	w.requestV2.Datapoint = &w.datapoint
}

func (w *writeTaggedOperation) Close() {
	p := w.pool
	w.reset()
	if p != nil {
		p.Put(w)
	}
}

func (w *writeTaggedOperation) Size() int {
	// Writes always represent a single write
	return 1
}

func (w *writeTaggedOperation) CompletionFn() completionFn {
	return w.completionFn
}

func (w *writeTaggedOperation) SetCompletionFn(fn completionFn) {
	w.completionFn = fn
}

func (w *writeTaggedOperation) ShardID() uint32 {
	return w.shardID
}

type writeTaggedOperationPool struct {
	pool pool.ObjectPool
}

func newWriteTaggedOpPool(opts pool.ObjectPoolOptions) *writeTaggedOperationPool {
	p := pool.NewObjectPool(opts)
	return &writeTaggedOperationPool{pool: p}
}

func (p *writeTaggedOperationPool) Init() {
	p.pool.Init(func() interface{} {
		w := &writeTaggedOperation{}
		w.reset()
		return w
	})
}

func (p *writeTaggedOperationPool) Get() *writeTaggedOperation {
	w := p.pool.Get().(*writeTaggedOperation)
	w.pool = p
	return w
}

func (p *writeTaggedOperationPool) Put(w *writeTaggedOperation) {
	w.reset()
	p.pool.Put(w)
}
