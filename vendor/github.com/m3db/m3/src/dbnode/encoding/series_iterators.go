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

type seriesIterators struct {
	iters  []SeriesIterator
	closed bool
	pool   MutableSeriesIteratorsPool
}

// NewSeriesIterators creates a new series iterators collection
func NewSeriesIterators(
	iters []SeriesIterator,
	pool MutableSeriesIteratorsPool,
) MutableSeriesIterators {
	return &seriesIterators{iters: iters}
}

func (iters *seriesIterators) Iters() []SeriesIterator {
	return iters.iters
}

func (iters *seriesIterators) Close() {
	if iters.closed {
		return
	}
	iters.closed = true
	for i := range iters.iters {
		if iters.iters[i] != nil {
			iters.iters[i].Close()
			iters.iters[i] = nil
		}
	}
	if iters.pool != nil {
		iters.pool.Put(iters)
	}
}

func (iters *seriesIterators) Len() int {
	return len(iters.iters)
}

func (iters *seriesIterators) Cap() int {
	return cap(iters.iters)
}

func (iters *seriesIterators) SetAt(idx int, iter SeriesIterator) {
	iters.iters[idx] = iter
}

func (iters *seriesIterators) Reset(size int) {
	for i := range iters.iters {
		iters.iters[i] = nil
	}
	iters.iters = iters.iters[:size]
}

// EmptySeriesIterators is an empty SeriesIterators.
var EmptySeriesIterators SeriesIterators = emptyIters{}

type emptyIters struct{}

func (e emptyIters) Iters() []SeriesIterator { return nil }
func (e emptyIters) Len() int                { return 0 }
func (e emptyIters) Close()                  {}
