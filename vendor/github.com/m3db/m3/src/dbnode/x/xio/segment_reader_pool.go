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

package xio

import (
	"github.com/m3db/m3/src/dbnode/ts"
	"github.com/m3db/m3/src/x/pool"
)

type segmentReaderPool struct {
	pool pool.ObjectPool
}

// NewSegmentReaderPool creates a new pool
func NewSegmentReaderPool(opts pool.ObjectPoolOptions) SegmentReaderPool {
	return &segmentReaderPool{pool: pool.NewObjectPool(opts)}
}

func (p *segmentReaderPool) Init() {
	p.pool.Init(func() interface{} {
		sr := NewSegmentReader(ts.Segment{}).(*segmentReader)
		sr.pool = p
		return sr
	})
}

func (p *segmentReaderPool) Get() SegmentReader {
	return p.pool.Get().(SegmentReader)
}

func (p *segmentReaderPool) Put(sr SegmentReader) {
	p.pool.Put(sr)
}
