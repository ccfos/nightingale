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

package postings

import (
	"errors"
	"math"
)

// ID is the unique identifier of an element in the postings list.
// TODO: Use a uint64, currently we only use uint32 because we use
// Roaring Bitmaps for the implementation of the postings list.
// Pilosa has an implementation of Roaring Bitmaps using uint64s.
type ID uint32

const (
	// MaxID is the maximum possible value for a postings ID.
	// TODO: Update these use uint64 when we update ID.
	MaxID ID = ID(math.MaxUint32)
)

var (
	// ErrEmptyList is the error returned when a postings list is unexpectedly empty.
	ErrEmptyList = errors.New("postings list is empty")
)

// List is a collection of docIDs. The interface only supports immutable methods.
type List interface {
	// Contains returns whether the specified ID is contained in this postings list.
	Contains(id ID) bool

	// IsEmpty returns whether the postings list is empty. Some posting lists have an
	// optimized implementation to determine if they are empty which is faster than
	// calculating the size of the postings list.
	IsEmpty() bool

	// Max returns the maximum ID in the postings list or an error if it is empty.
	Max() (ID, error)

	// Len returns the numbers of IDs in the postings list.
	Len() int

	// Iterator returns an iterator over the IDs in the postings list.
	Iterator() Iterator

	// Clone returns a copy of the postings list.
	Clone() MutableList

	// Equal returns whether this postings list contains the same posting IDs as other.
	Equal(other List) bool
}

// MutableList is a postings list implementation which also supports mutable operations.
type MutableList interface {
	List

	// Insert inserts the given ID into the postings list.
	Insert(i ID) error

	// Intersect updates this postings list in place to contain only those DocIDs which are
	// in both this postings list and other.
	Intersect(other List) error

	// Difference updates this postings list in place to contain only those DocIDs which are
	// in this postings list but not other.
	Difference(other List) error

	// Union updates this postings list in place to contain those DocIDs which are in either
	// this postings list or other.
	Union(other List) error

	// UnionMany updates this postings list in place to contain those DocIDs which are in
	// either this postings list or multiple others.
	UnionMany(others []List) error

	// AddIterator adds all IDs contained in the iterator.
	AddIterator(iter Iterator) error

	// AddRange adds all IDs between [min, max) to this postings list.
	AddRange(min, max ID) error

	// RemoveRange removes all IDs between [min, max) from this postings list.
	RemoveRange(min, max ID) error

	// Reset resets the internal state of the postings list.
	Reset()
}

// Iterator is an iterator over a postings list. The iterator is guaranteed to return
// IDs in increasing order. It is not safe for concurrent access.
type Iterator interface {
	// Next returns whether the iterator has another postings ID.
	Next() bool

	// Current returns the current postings ID. It is only safe to call Current immediately
	// after a call to Next confirms there are more IDs remaining.
	Current() ID

	// Err returns any errors encountered during iteration.
	Err() error

	// Close closes the iterator.
	Close() error
}

// Pool provides a pool for MutableLists.
type Pool interface {
	// Get retrieves a MutableList.
	Get() MutableList

	// Put releases the provided MutableList back to the pool.
	Put(pl MutableList)
}
