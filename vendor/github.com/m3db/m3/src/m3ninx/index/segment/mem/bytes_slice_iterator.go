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

package mem

import (
	"bytes"
	"sort"

	sgmt "github.com/m3db/m3/src/m3ninx/index/segment"
)

type bytesSliceIter struct {
	err  error
	done bool

	currentIdx   int
	current      []byte
	backingSlice [][]byte
	opts         Options
}

var _ sgmt.FieldsIterator = &bytesSliceIter{}

func newBytesSliceIter(slice [][]byte, opts Options) *bytesSliceIter {
	sortSliceOfByteSlices(slice)
	return &bytesSliceIter{
		currentIdx:   -1,
		backingSlice: slice,
		opts:         opts,
	}
}

func (b *bytesSliceIter) Next() bool {
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

func (b *bytesSliceIter) Current() []byte {
	return b.current
}

func (b *bytesSliceIter) Err() error {
	return nil
}

func (b *bytesSliceIter) Len() int {
	return len(b.backingSlice)
}

func (b *bytesSliceIter) Close() error {
	b.current = nil
	b.opts.BytesSliceArrayPool().Put(b.backingSlice)
	return nil
}

func sortSliceOfByteSlices(b [][]byte) {
	sort.Slice(b, func(i, j int) bool {
		return bytes.Compare(b[i], b[j]) < 0
	})
}
