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

package encoding

import (
	"github.com/m3db/m3/src/x/pool"
)

type readerIteratorPool struct {
	pool pool.ObjectPool
}

// NewReaderIteratorPool creates a new pool for ReaderIterators.
func NewReaderIteratorPool(opts pool.ObjectPoolOptions) ReaderIteratorPool {
	return &readerIteratorPool{pool: pool.NewObjectPool(opts)}
}

func (p *readerIteratorPool) Init(alloc ReaderIteratorAllocate) {
	p.pool.Init(func() interface{} {
		return alloc(nil, nil)
	})
}

func (p *readerIteratorPool) Get() ReaderIterator {
	return p.pool.Get().(ReaderIterator)
}

func (p *readerIteratorPool) Put(iter ReaderIterator) {
	p.pool.Put(iter)
}

type multiReaderIteratorPool struct {
	pool pool.ObjectPool
}

// NewMultiReaderIteratorPool creates a new pool for MultiReaderIterators.
func NewMultiReaderIteratorPool(opts pool.ObjectPoolOptions) MultiReaderIteratorPool {
	return &multiReaderIteratorPool{pool: pool.NewObjectPool(opts)}
}

func (p *multiReaderIteratorPool) Init(alloc ReaderIteratorAllocate) {
	p.pool.Init(func() interface{} {
		return NewMultiReaderIterator(alloc, p)
	})
}

func (p *multiReaderIteratorPool) Get() MultiReaderIterator {
	return p.pool.Get().(MultiReaderIterator)
}

func (p *multiReaderIteratorPool) Put(iter MultiReaderIterator) {
	p.pool.Put(iter)
}
