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

package block

import (
	"github.com/m3db/m3/src/x/pool"
)

type databaseBlockPool struct {
	pool pool.ObjectPool
}

// NewDatabaseBlockPool creates a new pool for database blocks.
func NewDatabaseBlockPool(opts pool.ObjectPoolOptions) DatabaseBlockPool {
	return &databaseBlockPool{pool: pool.NewObjectPool(opts)}
}

func (p *databaseBlockPool) Init(alloc DatabaseBlockAllocate) {
	p.pool.Init(func() interface{} {
		return alloc()
	})
}

func (p *databaseBlockPool) Get() DatabaseBlock {
	return p.pool.Get().(DatabaseBlock)
}

func (p *databaseBlockPool) Put(block DatabaseBlock) {
	p.pool.Put(block)
}
