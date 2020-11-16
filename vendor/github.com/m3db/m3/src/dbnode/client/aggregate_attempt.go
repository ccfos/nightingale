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
	"github.com/m3db/m3/src/dbnode/storage/index"
	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/m3/src/x/pool"
	xretry "github.com/m3db/m3/src/x/retry"
)

var (
	aggregateAttemptArgsZeroed aggregateAttemptArgs
)

type aggregateAttempt struct {
	session   *session
	attemptFn xretry.Fn

	args           aggregateAttemptArgs
	resultIter     AggregatedTagsIterator
	resultMetadata FetchResponseMetadata
}

type aggregateAttemptArgs struct {
	ns    ident.ID
	query index.Query
	opts  index.AggregationOptions
}

func (f *aggregateAttempt) reset() {
	f.args = aggregateAttemptArgsZeroed
	f.resultIter = nil
	f.resultMetadata = FetchResponseMetadata{}
}

func (f *aggregateAttempt) performAttempt() error {
	var err error
	f.resultIter, f.resultMetadata, err = f.session.aggregateAttempt(
		f.args.ns, f.args.query, f.args.opts)
	return err
}

type aggregateAttemptPool interface {
	Init()
	Get() *aggregateAttempt
	Put(*aggregateAttempt)
}

type aggregateAttemptPoolImpl struct {
	pool    pool.ObjectPool
	session *session
}

func newAggregateAttemptPool(
	session *session,
	opts pool.ObjectPoolOptions,
) aggregateAttemptPool {
	p := pool.NewObjectPool(opts)
	return &aggregateAttemptPoolImpl{pool: p, session: session}
}

func (p *aggregateAttemptPoolImpl) Init() {
	p.pool.Init(func() interface{} {
		f := &aggregateAttempt{session: p.session}
		// NB(prateek): Bind fn once to avoid creating receiver
		// and function method pointer over and over again
		f.attemptFn = f.performAttempt
		f.reset()
		return f
	})
}

func (p *aggregateAttemptPoolImpl) Get() *aggregateAttempt {
	return p.pool.Get().(*aggregateAttempt)
}

func (p *aggregateAttemptPoolImpl) Put(f *aggregateAttempt) {
	f.reset()
	p.pool.Put(f)
}
