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

package serialize

import (
	"bytes"

	"github.com/m3db/m3/src/x/checked"
	"github.com/m3db/m3/src/x/pool"
)

type metricTagsIter struct {
	tagDecoder TagDecoder
	bytes      checked.Bytes
	pool       MetricTagsIteratorPool
}

// NewMetricTagsIterator creates a MetricTagsIterator.
func NewMetricTagsIterator(
	tagDecoder TagDecoder,
	pool MetricTagsIteratorPool,
) MetricTagsIterator {
	return &metricTagsIter{
		tagDecoder: tagDecoder,
		bytes:      checked.NewBytes(nil, nil),
		pool:       pool,
	}
}

func (it *metricTagsIter) Reset(sortedTagPairs []byte) {
	it.bytes.IncRef()
	it.bytes.Reset(sortedTagPairs)
	it.tagDecoder.Reset(it.bytes)
}

func (it *metricTagsIter) Bytes() []byte {
	return it.bytes.Bytes()
}

func (it *metricTagsIter) NumTags() int {
	return it.tagDecoder.Len()
}

func (it *metricTagsIter) TagValue(tagName []byte) ([]byte, bool) {
	iter := it.tagDecoder.Duplicate()
	defer iter.Close()

	for iter.Next() {
		tag := iter.Current()
		if bytes.Equal(tagName, tag.Name.Bytes()) {
			return tag.Value.Bytes(), true
		}
	}
	return nil, false
}

func (it *metricTagsIter) Next() bool {
	return it.tagDecoder.Next()
}

func (it *metricTagsIter) Current() ([]byte, []byte) {
	tag := it.tagDecoder.Current()
	return tag.Name.Bytes(), tag.Value.Bytes()
}

func (it *metricTagsIter) Err() error {
	return it.tagDecoder.Err()
}

func (it *metricTagsIter) Close() {
	it.bytes.Reset(nil)
	it.bytes.DecRef()
	it.tagDecoder.Reset(it.bytes)

	if it.pool != nil {
		it.pool.Put(it)
	}
}

type metricTagsIteratorPool struct {
	tagDecoderPool TagDecoderPool
	pool           pool.ObjectPool
}

// NewMetricTagsIteratorPool creates a MetricTagsIteratorPool.
func NewMetricTagsIteratorPool(
	tagDecoderPool TagDecoderPool,
	opts pool.ObjectPoolOptions,
) MetricTagsIteratorPool {
	return &metricTagsIteratorPool{
		tagDecoderPool: tagDecoderPool,
		pool:           pool.NewObjectPool(opts),
	}
}

// Init initializes the pool.
func (p *metricTagsIteratorPool) Init() {
	p.pool.Init(func() interface{} {
		return NewMetricTagsIterator(p.tagDecoderPool.Get(), p)
	})
}

// Get provides a MetricTagsIterator from the pool.
func (p *metricTagsIteratorPool) Get() MetricTagsIterator {
	return p.pool.Get().(*metricTagsIter)
}

// Put returns a MetricTagsIterator to the pool.
func (p *metricTagsIteratorPool) Put(v MetricTagsIterator) {
	p.pool.Put(v)
}
