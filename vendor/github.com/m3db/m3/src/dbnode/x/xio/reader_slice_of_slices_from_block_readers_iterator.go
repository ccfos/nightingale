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
	"time"
)

type readerSliceOfSlicesIterator struct {
	blocks [][]BlockReader
	idx    int
	len    int
	closed bool
}

// NewReaderSliceOfSlicesFromBlockReadersIterator creates a new reader slice of slices iterator
func NewReaderSliceOfSlicesFromBlockReadersIterator(
	blocks [][]BlockReader,
) ReaderSliceOfSlicesFromBlockReadersIterator {
	it := &readerSliceOfSlicesIterator{}
	it.Reset(blocks)
	return it
}

func (it *readerSliceOfSlicesIterator) Next() bool {
	if it.idx >= it.len-1 {
		return false
	}
	it.idx++
	return true
}

var timeZero = time.Time{}

// ensure readerSliceOfSlicesIterator implements ReaderSliceOfSlicesIterator
var _ ReaderSliceOfSlicesIterator = &readerSliceOfSlicesIterator{}

func (it *readerSliceOfSlicesIterator) CurrentReaders() (int, time.Time, time.Duration) {
	if len(it.blocks) < it.arrayIdx() {
		return 0, timeZero, 0
	}
	currentLen := len(it.blocks[it.arrayIdx()])
	if currentLen == 0 {
		return 0, timeZero, 0
	}
	currBlock := it.blocks[it.arrayIdx()][0]
	return currentLen, currBlock.Start, currBlock.BlockSize
}

func (it *readerSliceOfSlicesIterator) CurrentReaderAt(idx int) BlockReader {
	return it.blocks[it.arrayIdx()][idx]
}

func (it *readerSliceOfSlicesIterator) Reset(blocks [][]BlockReader) {
	it.blocks = blocks
	it.resetIndex()
	it.len = len(blocks)
	it.closed = false
}

func (it *readerSliceOfSlicesIterator) Close() {
	if it.closed {
		return
	}
	it.closed = true
}

func (it *readerSliceOfSlicesIterator) arrayIdx() int {
	idx := it.idx
	if idx == -1 {
		idx = 0
	}
	return idx
}

func (it *readerSliceOfSlicesIterator) Size() (int, error) {
	size := 0
	for _, blocks := range it.blocks {
		for _, blockAtTime := range blocks {
			seg, err := blockAtTime.Segment()
			if err != nil {
				return 0, err
			}
			size += seg.Len()
		}
	}
	return size, nil
}

func (it *readerSliceOfSlicesIterator) Rewind() {
	it.resetIndex()
}

func (it *readerSliceOfSlicesIterator) resetIndex() {
	it.idx = -1
}
