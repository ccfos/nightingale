// Copyright (c) 2020 Uber Technologies, Inc.
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

package mem

import (
	"bytes"
	"sort"

	sgmt "github.com/m3db/m3/src/m3ninx/index/segment"
	"github.com/m3db/m3/src/m3ninx/postings"
)

type uniqueField struct {
	field        []byte
	postingsList postings.List
}

type uniqueFieldsIter struct {
	err  error
	done bool

	currentIdx   int
	current      uniqueField
	backingSlice []uniqueField
	opts         Options
}

var _ sgmt.FieldsPostingsListIterator = &uniqueFieldsIter{}

func newUniqueFieldsIter(slice []uniqueField, opts Options) *uniqueFieldsIter {
	sortSliceOfUniqueFields(slice)
	return &uniqueFieldsIter{
		currentIdx:   -1,
		backingSlice: slice,
		opts:         opts,
	}
}

func (b *uniqueFieldsIter) Next() bool {
	if b.done || b.err != nil {
		return false
	}
	b.currentIdx++
	if b.currentIdx >= len(b.backingSlice) {
		b.done = true
		return false
	}
	b.current = b.backingSlice[b.currentIdx]
	return true
}

func (b *uniqueFieldsIter) Current() ([]byte, postings.List) {
	return b.current.field, b.current.postingsList
}

func (b *uniqueFieldsIter) Err() error {
	return nil
}

func (b *uniqueFieldsIter) Len() int {
	return len(b.backingSlice)
}

func (b *uniqueFieldsIter) Close() error {
	b.current = uniqueField{}
	return nil
}

func sortSliceOfUniqueFields(b []uniqueField) {
	sort.Slice(b, func(i, j int) bool {
		return bytes.Compare(b[i].field, b[j].field) < 0
	})
}
