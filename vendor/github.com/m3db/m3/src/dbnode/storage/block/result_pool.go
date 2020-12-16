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

type fetchBlockMetadataResultsPool struct {
	pool     pool.ObjectPool
	capacity int
}

// NewFetchBlockMetadataResultsPool creates a new fetchBlockMetadataResultsPool
func NewFetchBlockMetadataResultsPool(opts pool.ObjectPoolOptions, capacity int) FetchBlockMetadataResultsPool {
	p := &fetchBlockMetadataResultsPool{pool: pool.NewObjectPool(opts), capacity: capacity}
	p.pool.Init(func() interface{} {
		res := make([]FetchBlockMetadataResult, 0, p.capacity)
		return newPooledFetchBlockMetadataResults(res, p)
	})
	return p
}

func (p *fetchBlockMetadataResultsPool) Get() FetchBlockMetadataResults {
	return p.pool.Get().(FetchBlockMetadataResults)
}

func (p *fetchBlockMetadataResultsPool) Put(res FetchBlockMetadataResults) {
	if res == nil || cap(res.Results()) > p.capacity {
		// Don't return nil or large slices back to pool
		return
	}
	res.Reset()
	p.pool.Put(res)
}

type fetchBlocksMetadataResultsPool struct {
	pool     pool.ObjectPool
	capacity int
}

// NewFetchBlocksMetadataResultsPool creates a new fetchBlocksMetadataResultsPool
func NewFetchBlocksMetadataResultsPool(opts pool.ObjectPoolOptions, capacity int) FetchBlocksMetadataResultsPool {
	p := &fetchBlocksMetadataResultsPool{pool: pool.NewObjectPool(opts), capacity: capacity}
	p.pool.Init(func() interface{} {
		res := make([]FetchBlocksMetadataResult, 0, capacity)
		return newPooledFetchBlocksMetadataResults(res, p)
	})
	return p
}

func (p *fetchBlocksMetadataResultsPool) Get() FetchBlocksMetadataResults {
	return p.pool.Get().(FetchBlocksMetadataResults)
}

func (p *fetchBlocksMetadataResultsPool) Put(res FetchBlocksMetadataResults) {
	if res == nil || cap(res.Results()) > p.capacity {
		// Don't return nil or large slices back to pool
		return
	}
	res.Reset()
	p.pool.Put(res)
}
