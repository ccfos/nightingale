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
	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/storage/index"
	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/m3/src/x/pool"
	xretry "github.com/m3db/m3/src/x/retry"
)

var (
	fetchTaggedAttemptArgsZeroed fetchTaggedAttemptArgs
)

type fetchTaggedAttempt struct {
	args    fetchTaggedAttemptArgs
	session *session

	idsAttemptFn       xretry.Fn
	dataAttemptFn      xretry.Fn
	idsResultIter      TaggedIDsIterator
	dataResultIters    encoding.SeriesIterators
	idsResultMetadata  FetchResponseMetadata
	dataResultMetadata FetchResponseMetadata
}

type fetchTaggedAttemptArgs struct {
	ns    ident.ID
	query index.Query
	opts  index.QueryOptions
}

func (f *fetchTaggedAttempt) reset() {
	f.args = fetchTaggedAttemptArgsZeroed

	f.idsResultIter = nil
	f.idsResultMetadata = FetchResponseMetadata{}
	f.dataResultIters = nil
	f.dataResultMetadata = FetchResponseMetadata{}
}

func (f *fetchTaggedAttempt) performIDsAttempt() error {
	var err error
	f.idsResultIter, f.idsResultMetadata, err = f.session.fetchTaggedIDsAttempt(
		f.args.ns, f.args.query, f.args.opts)
	return err
}

func (f *fetchTaggedAttempt) performDataAttempt() error {
	var err error
	f.dataResultIters, f.dataResultMetadata, err = f.session.fetchTaggedAttempt(
		f.args.ns, f.args.query, f.args.opts)
	return err
}

type fetchTaggedAttemptPool interface {
	Init()
	Get() *fetchTaggedAttempt
	Put(*fetchTaggedAttempt)
}

type fetchTaggedAttemptPoolImpl struct {
	pool    pool.ObjectPool
	session *session
}

func newFetchTaggedAttemptPool(
	session *session,
	opts pool.ObjectPoolOptions,
) fetchTaggedAttemptPool {
	p := pool.NewObjectPool(opts)
	return &fetchTaggedAttemptPoolImpl{pool: p, session: session}
}

func (p *fetchTaggedAttemptPoolImpl) Init() {
	p.pool.Init(func() interface{} {
		f := &fetchTaggedAttempt{session: p.session}
		// NB(prateek): Bind fn once to avoid creating receiver
		// and function method pointer over and over again
		f.idsAttemptFn = f.performIDsAttempt
		f.dataAttemptFn = f.performDataAttempt
		f.reset()
		return f
	})
}

func (p *fetchTaggedAttemptPoolImpl) Get() *fetchTaggedAttempt {
	return p.pool.Get().(*fetchTaggedAttempt)
}

func (p *fetchTaggedAttemptPoolImpl) Put(f *fetchTaggedAttempt) {
	f.reset()
	p.pool.Put(f)
}
