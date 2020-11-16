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
	"time"

	"github.com/m3db/m3/src/dbnode/ts"
	"github.com/m3db/m3/src/x/ident"
	xtime "github.com/m3db/m3/src/x/time"
)

type seriesIterator struct {
	id               ident.ID
	nsID             ident.ID
	tags             ident.TagIterator
	start            xtime.UnixNano
	end              xtime.UnixNano
	iters            iterators
	multiReaderIters []MultiReaderIterator
	err              error
	firstNext        bool
	closed           bool
	pool             SeriesIteratorPool
}

// NewSeriesIterator creates a new series iterator.
// NB: The returned SeriesIterator assumes ownership of the provided `ident.ID`.
func NewSeriesIterator(
	opts SeriesIteratorOptions,
	pool SeriesIteratorPool,
) SeriesIterator {
	it := &seriesIterator{pool: pool}
	it.Reset(opts)
	return it
}

func (it *seriesIterator) ID() ident.ID {
	return it.id
}

func (it *seriesIterator) Namespace() ident.ID {
	return it.nsID
}

func (it *seriesIterator) Tags() ident.TagIterator {
	return it.tags
}

func (it *seriesIterator) Start() time.Time {
	return it.start.ToTime()
}

func (it *seriesIterator) End() time.Time {
	return it.end.ToTime()
}

func (it *seriesIterator) Next() bool {
	if !it.firstNext {
		if !it.hasNext() {
			return false
		}
		it.moveToNext()
	}
	it.firstNext = false
	return it.hasNext()
}

func (it *seriesIterator) Current() (ts.Datapoint, xtime.Unit, ts.Annotation) {
	return it.iters.current()
}

func (it *seriesIterator) Err() error {
	return it.err
}

func (it *seriesIterator) Close() {
	if it.isClosed() {
		return
	}
	it.closed = true
	if it.id != nil {
		it.id.Finalize()
		it.id = nil
	}
	if it.nsID != nil {
		it.nsID.Finalize()
		it.nsID = nil
	}
	if it.tags != nil {
		it.tags.Close()
		it.tags = nil
	}

	for idx := range it.multiReaderIters {
		it.multiReaderIters[idx] = nil
	}

	it.iters.reset()
	if it.pool != nil {
		it.pool.Put(it)
	}
}

func (it *seriesIterator) Replicas() ([]MultiReaderIterator, error) {
	return it.multiReaderIters, nil
}

func (it *seriesIterator) Reset(opts SeriesIteratorOptions) {
	it.id = opts.ID
	it.nsID = opts.Namespace
	it.tags = opts.Tags
	if it.tags == nil {
		it.tags = ident.EmptyTagIterator
	}
	it.multiReaderIters = it.multiReaderIters[:0]
	it.err = nil
	it.firstNext = true
	it.closed = false

	it.iters.reset()
	it.start = opts.StartInclusive
	it.end = opts.EndExclusive
	if it.start != 0 && it.end != 0 {
		it.iters.setFilter(it.start, it.end)
	}
	it.SetIterateEqualTimestampStrategy(opts.IterateEqualTimestampStrategy)
	replicas := opts.Replicas
	var err error
	if consolidator := opts.SeriesIteratorConsolidator; consolidator != nil {
		replicas, err = consolidator.ConsolidateReplicas(replicas)
		if err != nil {
			it.err = err
			return
		}
	}

	for _, replica := range replicas {
		if !replica.Next() || !it.iters.push(replica) {
			if replica.Err() != nil {
				it.err = replica.Err()
			}

			replica.Close()
			continue
		}
		it.multiReaderIters = append(it.multiReaderIters, replica)
	}
}

func (it *seriesIterator) SetIterateEqualTimestampStrategy(strategy IterateEqualTimestampStrategy) {
	it.iters.equalTimesStrategy = strategy
}

func (it *seriesIterator) hasError() bool {
	return it.err != nil
}

func (it *seriesIterator) isClosed() bool {
	return it.closed
}

func (it *seriesIterator) hasMore() bool {
	return it.iters.len() > 0
}

func (it *seriesIterator) hasNext() bool {
	return !it.hasError() && !it.isClosed() && it.hasMore()
}

func (it *seriesIterator) moveToNext() {
	for {
		prev := it.iters.at()
		next, err := it.iters.moveToValidNext()
		if err != nil {
			it.err = err
			return
		}
		if !next {
			return
		}

		curr := it.iters.at()
		if curr != prev {
			return
		}

		// Dedupe by continuing
	}
}

func (it *seriesIterator) Stats() (SeriesIteratorStats, error) {
	approx := 0
	for _, readerIter := range it.multiReaderIters {
		readers := readerIter.Readers()
		size, err := readers.Size()
		if err != nil {
			return SeriesIteratorStats{}, err
		}
		approx += size
	}
	return SeriesIteratorStats{ApproximateSizeInBytes: approx}, nil
}
