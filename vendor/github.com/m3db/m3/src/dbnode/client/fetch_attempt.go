// Copyright (c) 2017 Uber Technologies, Inc.
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
	"time"

	"github.com/m3db/m3/src/dbnode/encoding"
	xerrors "github.com/m3db/m3/src/x/errors"
	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/m3/src/x/pool"
	xretry "github.com/m3db/m3/src/x/retry"
)

var fetchAttemptArgsZeroed fetchAttemptArgs

type fetchAttempt struct {
	args fetchAttemptArgs

	result encoding.SeriesIterators

	session *session

	attemptFn xretry.Fn
}

type fetchAttemptArgs struct {
	namespace ident.ID
	ids       ident.Iterator
	start     time.Time
	end       time.Time
}

func (f *fetchAttempt) reset() {
	f.args = fetchAttemptArgsZeroed
	f.result = nil
}

func (f *fetchAttempt) perform() error {
	result, err := f.session.fetchIDsAttempt(f.args.namespace,
		f.args.ids, f.args.start, f.args.end)
	f.result = result

	if IsBadRequestError(err) {
		// Do not retry bad request errors
		err = xerrors.NewNonRetryableError(err)
	}

	return err
}

type fetchAttemptPool struct {
	pool    pool.ObjectPool
	session *session
}

func newFetchAttemptPool(
	session *session,
	opts pool.ObjectPoolOptions,
) *fetchAttemptPool {
	p := pool.NewObjectPool(opts)
	return &fetchAttemptPool{pool: p, session: session}
}

func (p *fetchAttemptPool) Init() {
	p.pool.Init(func() interface{} {
		w := &fetchAttempt{session: p.session}
		// NB(r): Bind attemptFn once to avoid creating receiver
		// and function method pointer over and over again
		w.attemptFn = w.perform
		w.reset()
		return w
	})
}

func (p *fetchAttemptPool) Get() *fetchAttempt {
	return p.pool.Get().(*fetchAttempt)
}

func (p *fetchAttemptPool) Put(f *fetchAttempt) {
	f.reset()
	p.pool.Put(f)
}
