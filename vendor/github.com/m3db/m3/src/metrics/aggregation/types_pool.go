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

package aggregation

import "github.com/m3db/m3/src/x/pool"

// TypesAlloc allocates new aggregation types.
type TypesAlloc func() Types

// TypesPool provides a pool of aggregation types.
type TypesPool interface {
	// Init initializes the aggregation types pool.
	Init(alloc TypesAlloc)

	// Get gets an empty list of aggregation types from the pool.
	Get() Types

	// Put returns aggregation types to the pool.
	Put(value Types)
}

type typesPool struct {
	pool pool.ObjectPool
}

// NewTypesPool creates a new pool for aggregation types.
func NewTypesPool(opts pool.ObjectPoolOptions) TypesPool {
	return &typesPool{pool: pool.NewObjectPool(opts)}
}

func (p *typesPool) Init(alloc TypesAlloc) {
	p.pool.Init(func() interface{} {
		return alloc()
	})
}

func (p *typesPool) Get() Types {
	return p.pool.Get().(Types)
}

func (p *typesPool) Put(value Types) {
	p.pool.Put(value[:0])
}
