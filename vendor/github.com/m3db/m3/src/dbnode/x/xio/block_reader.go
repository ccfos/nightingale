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

package xio

import (
	"time"

	"github.com/m3db/m3/src/dbnode/ts"
	"github.com/m3db/m3/src/x/pool"
)

// CloneBlock returns a clone of the block with the underlying data reset
func (b BlockReader) CloneBlock(pool pool.CheckedBytesPool) (BlockReader, error) {
	sr, err := b.SegmentReader.Clone(pool)
	if err != nil {
		return EmptyBlockReader, err
	}
	return BlockReader{
		SegmentReader: sr,
		Start:         b.Start,
		BlockSize:     b.BlockSize,
	}, nil
}

// IsEmpty returns true for the empty block
func (b BlockReader) IsEmpty() bool {
	return b.SegmentReader == nil && b.Start.Equal(timeZero) && b.BlockSize == 0
}

// IsNotEmpty returns false for the empty block
func (b BlockReader) IsNotEmpty() bool {
	return !b.IsEmpty()
}

// ResetWindowed resets the underlying reader window, as well as start time and blockSize for the block
func (b *BlockReader) ResetWindowed(segment ts.Segment, start time.Time, blockSize time.Duration) {
	b.Reset(segment)
	b.Start = start
	b.BlockSize = blockSize
}

// FilterEmptyBlockReadersSliceOfSlicesInPlace filters a [][]BlockReader in place (I.E by modifying
// the existing data structures instead of allocating new ones) such that the returned [][]BlockReader
// will only contain BlockReaders that contain non-empty segments.
//
// Note that if any of the Block/Segment readers are backed by async implementations then this function
// will not return until all of the async execution has completed.
func FilterEmptyBlockReadersSliceOfSlicesInPlace(brSliceOfSlices [][]BlockReader) ([][]BlockReader, error) {
	filteredSliceOfSlices := brSliceOfSlices[:0]
	for _, brSlice := range brSliceOfSlices {
		filteredBrSlice, err := FilterEmptyBlockReadersInPlace(brSlice)
		if err != nil {
			return nil, err
		}
		if len(filteredBrSlice) > 0 {
			filteredSliceOfSlices = append(filteredSliceOfSlices, filteredBrSlice)
		}
	}
	return filteredSliceOfSlices, nil
}

// FilterEmptyBlockReadersInPlace is the same as FilterEmptyBlockReadersSliceOfSlicesInPlace except for
// one dimensional slices instead of two.
func FilterEmptyBlockReadersInPlace(brs []BlockReader) ([]BlockReader, error) {
	filtered := brs[:0]
	for _, br := range brs {
		if br.SegmentReader == nil {
			continue
		}
		segment, err := br.Segment()
		if err != nil {
			return nil, err
		}
		if segment.Len() > 0 {
			filtered = append(filtered, br)
		}
	}
	return filtered, nil
}
