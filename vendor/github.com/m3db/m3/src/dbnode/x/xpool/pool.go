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

package xpool

import (
	"github.com/m3db/m3/src/x/checked"
	"github.com/m3db/m3/src/x/pool"
)

type checkedPool struct {
	pool pool.ObjectPool
	opts checked.BytesOptions
}

// NewCheckedBytesWrapperPool returns a new CheckedBytesWrapperPool with the
// defined size.
func NewCheckedBytesWrapperPool(popts pool.ObjectPoolOptions) CheckedBytesWrapperPool {
	p := pool.NewObjectPool(popts)
	opts := checked.NewBytesOptions().SetFinalizer(
		checked.BytesFinalizerFn(func(b checked.Bytes) {
			b.IncRef()
			b.Reset(nil)
			b.DecRef()
			p.Put(b)
		}))

	return &checkedPool{
		pool: p,
		opts: opts,
	}
}

func (p *checkedPool) Init() {
	p.pool.Init(func() interface{} {
		return checked.NewBytes(nil, p.opts)
	})
}

func (p *checkedPool) Get(b []byte) checked.Bytes {
	checkedBytes := p.pool.Get().(checked.Bytes)
	checkedBytes.IncRef()
	checkedBytes.Reset(b)
	checkedBytes.DecRef()
	return checkedBytes
}
