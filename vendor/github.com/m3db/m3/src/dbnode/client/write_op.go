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
	"math"

	"github.com/m3db/m3/src/dbnode/generated/thrift/rpc"
	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/m3/src/x/pool"
)

var (
	// NB(bl): use an invalid shardID for the zerod op
	writeOperationZeroed = writeOperation{shardID: math.MaxUint32}
)

type writeOperation struct {
	namespace    ident.ID
	shardID      uint32
	request      rpc.WriteBatchRawRequestElement
	requestV2    rpc.WriteBatchRawV2RequestElement
	datapoint    rpc.Datapoint
	completionFn completionFn
	pool         *writeOperationPool
}

func (w *writeOperation) reset() {
	*w = writeOperationZeroed
	w.request.Datapoint = &w.datapoint
	w.requestV2.Datapoint = &w.datapoint
}

func (w *writeOperation) Close() {
	p := w.pool
	w.reset()
	if p != nil {
		p.Put(w)
	}
}

func (w *writeOperation) Size() int {
	// Writes always represent a single write
	return 1
}

func (w *writeOperation) CompletionFn() completionFn {
	return w.completionFn
}

func (w *writeOperation) SetCompletionFn(fn completionFn) {
	w.completionFn = fn
}

func (w *writeOperation) ShardID() uint32 {
	return w.shardID
}

type writeOperationPool struct {
	pool pool.ObjectPool
}

func newWriteOperationPool(opts pool.ObjectPoolOptions) *writeOperationPool {
	p := pool.NewObjectPool(opts)
	return &writeOperationPool{pool: p}
}

func (p *writeOperationPool) Init() {
	p.pool.Init(func() interface{} {
		w := &writeOperation{}
		w.reset()
		return w
	})
}

func (p *writeOperationPool) Get() *writeOperation {
	w := p.pool.Get().(*writeOperation)
	w.pool = p
	return w
}

func (p *writeOperationPool) Put(w *writeOperation) {
	w.reset()
	p.pool.Put(w)
}
