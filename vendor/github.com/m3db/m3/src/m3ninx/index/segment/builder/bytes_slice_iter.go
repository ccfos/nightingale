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

package builder

import (
	"bytes"

	"github.com/m3db/m3/src/m3ninx/index/segment"
	"github.com/m3db/m3/src/m3ninx/postings"

	"github.com/twotwotwo/sorts"
)

type uniqueField struct {
	field        []byte
	postingsList postings.List
}

// orderedFieldsPostingsListIter is a new ordered fields/postings list iterator.
type orderedFieldsPostingsListIter struct {
	err  error
	done bool

	currentIdx    int
	current       uniqueField
	backingSlices *sortableSliceOfSliceOfUniqueFieldsAsc
}

var _ segment.FieldsPostingsListIterator = &orderedFieldsPostingsListIter{}

// newOrderedFieldsPostingsListIter sorts a slice of slices of unique fields and then
// returns an iterator over them.
func newOrderedFieldsPostingsListIter(
	maybeUnorderedFields [][]uniqueField,
) *orderedFieldsPostingsListIter {
	sortable := &sortableSliceOfSliceOfUniqueFieldsAsc{data: maybeUnorderedFields}
	// NB(r): See SetSortConcurrency why this RLock is required.
	sortConcurrencyLock.RLock()
	sorts.ByBytes(sortable)
	sortConcurrencyLock.RUnlock()
	return &orderedFieldsPostingsListIter{
		currentIdx:    -1,
		backingSlices: sortable,
	}
}

// Next returns true if there is a next result.
func (b *orderedFieldsPostingsListIter) Next() bool {
	if b.done || b.err != nil {
		return false
	}
	b.currentIdx++
	if b.currentIdx >= b.backingSlices.Len() {
		b.done = true
		return false
	}
	iOuter, iInner := b.backingSlices.getIndices(b.currentIdx)
	b.current = b.backingSlices.data[iOuter][iInner]
	return true
}

// Current returns the current entry.
func (b *orderedFieldsPostingsListIter) Current() ([]byte, postings.List) {
	return b.current.field, b.current.postingsList
}

// Err returns an error if an error occurred iterating.
func (b *orderedFieldsPostingsListIter) Err() error {
	return nil
}

// Len returns the length of the slice.
func (b *orderedFieldsPostingsListIter) Len() int {
	return b.backingSlices.Len()
}

// Close releases resources.
func (b *orderedFieldsPostingsListIter) Close() error {
	b.current = uniqueField{}
	return nil
}

type sortableSliceOfSliceOfUniqueFieldsAsc struct {
	data   [][]uniqueField
	length int
}

func (s *sortableSliceOfSliceOfUniqueFieldsAsc) Len() int {
	if s.length > 0 {
		return s.length
	}

	totalLen := 0
	for _, innerSlice := range s.data {
		totalLen += len(innerSlice)
	}
	s.length = totalLen

	return s.length
}

func (s *sortableSliceOfSliceOfUniqueFieldsAsc) Less(i, j int) bool {
	iOuter, iInner := s.getIndices(i)
	jOuter, jInner := s.getIndices(j)
	return bytes.Compare(s.data[iOuter][iInner].field, s.data[jOuter][jInner].field) < 0
}

func (s *sortableSliceOfSliceOfUniqueFieldsAsc) Swap(i, j int) {
	iOuter, iInner := s.getIndices(i)
	jOuter, jInner := s.getIndices(j)
	s.data[iOuter][iInner], s.data[jOuter][jInner] = s.data[jOuter][jInner], s.data[iOuter][iInner]
}

func (s *sortableSliceOfSliceOfUniqueFieldsAsc) Key(i int) []byte {
	iOuter, iInner := s.getIndices(i)
	return s.data[iOuter][iInner].field
}

func (s *sortableSliceOfSliceOfUniqueFieldsAsc) getIndices(idx int) (int, int) {
	currentSliceIdx := 0
	for idx >= len(s.data[currentSliceIdx]) {
		idx -= len(s.data[currentSliceIdx])
		currentSliceIdx++
	}
	return currentSliceIdx, idx
}
