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

package series

import "github.com/m3db/m3/src/x/pool"

type databaseSeriesPool struct {
	pool pool.ObjectPool
}

// NewDatabaseSeriesPool creates a new database series pool
func NewDatabaseSeriesPool(opts pool.ObjectPoolOptions) DatabaseSeriesPool {
	p := &databaseSeriesPool{pool: pool.NewObjectPool(opts)}
	p.pool.Init(func() interface{} {
		return newPooledDatabaseSeries(p)
	})
	return p
}

func (p *databaseSeriesPool) Get() DatabaseSeries {
	return p.pool.Get().(DatabaseSeries)
}

func (p *databaseSeriesPool) Put(series DatabaseSeries) {
	p.pool.Put(series)
}
