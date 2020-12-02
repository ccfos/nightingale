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

package id

import (
	"bytes"

	"github.com/m3db/m3/src/x/pool"
)

// TagPair contains a tag name and a tag value.
type TagPair struct {
	Name  []byte
	Value []byte
}

// TagPairsByNameAsc sorts the tag pairs by tag names in ascending order.
type TagPairsByNameAsc []TagPair

func (tp TagPairsByNameAsc) Len() int      { return len(tp) }
func (tp TagPairsByNameAsc) Swap(i, j int) { tp[i], tp[j] = tp[j], tp[i] }

func (tp TagPairsByNameAsc) Less(i, j int) bool {
	return bytes.Compare(tp[i].Name, tp[j].Name) < 0
}

// SortedTagIteratorFn creates a new sorted tag iterator given id tag pairs.
type SortedTagIteratorFn func(tagPairs []byte) SortedTagIterator

// SortedTagIterator iterates over a set of tag pairs sorted by tag names.
type SortedTagIterator interface {
	// Reset resets the iterator.
	Reset(sortedTagPairs []byte)

	// Next returns true if there are more tag names and values.
	Next() bool

	// Current returns the current tag name and value.
	Current() ([]byte, []byte)

	// Err returns any errors encountered.
	Err() error

	// Close closes the iterator.
	Close()
}

// SortedTagIteratorAlloc allocates a new sorted tag iterator.
type SortedTagIteratorAlloc func() SortedTagIterator

// SortedTagIteratorPool is a pool of sorted tag iterators.
type SortedTagIteratorPool interface {
	// Init initializes the iterator pool.
	Init(alloc SortedTagIteratorAlloc)

	// Get gets an iterator from the pool.
	Get() SortedTagIterator

	// Put returns an iterator to the pool.
	Put(value SortedTagIterator)
}

type sortedTagIteratorPool struct {
	pool pool.ObjectPool
}

// NewSortedTagIteratorPool creates a new pool for sorted tag iterators.
func NewSortedTagIteratorPool(opts pool.ObjectPoolOptions) SortedTagIteratorPool {
	return &sortedTagIteratorPool{pool: pool.NewObjectPool(opts)}
}

func (p *sortedTagIteratorPool) Init(alloc SortedTagIteratorAlloc) {
	p.pool.Init(func() interface{} {
		return alloc()
	})
}

func (p *sortedTagIteratorPool) Get() SortedTagIterator {
	return p.pool.Get().(SortedTagIterator)
}

func (p *sortedTagIteratorPool) Put(it SortedTagIterator) {
	p.pool.Put(it)
}
