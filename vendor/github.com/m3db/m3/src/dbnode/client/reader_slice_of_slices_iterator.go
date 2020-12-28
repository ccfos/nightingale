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

package client

import (
	"time"

	"github.com/m3db/m3/src/dbnode/generated/thrift/rpc"
	"github.com/m3db/m3/src/dbnode/ts"
	"github.com/m3db/m3/src/dbnode/x/xio"
	"github.com/m3db/m3/src/x/checked"
	xtime "github.com/m3db/m3/src/x/time"
)

var timeZero = time.Time{}

type readerSliceOfSlicesIterator struct {
	segments     []*rpc.Segments
	blockReaders []xio.BlockReader
	idx          int
	closed       bool
	pool         *readerSliceOfSlicesIteratorPool
}

func newReaderSliceOfSlicesIterator(
	segments []*rpc.Segments,
	pool *readerSliceOfSlicesIteratorPool,
) *readerSliceOfSlicesIterator {
	it := &readerSliceOfSlicesIterator{pool: pool}
	it.Reset(segments)
	return it
}

func (it *readerSliceOfSlicesIterator) Next() bool {
	if !(it.idx+1 < len(it.segments)) {
		return false
	}
	it.idx++

	// Extend block readers if not enough available
	currLen, start, blockSize := it.CurrentReaders()
	if len(it.blockReaders) < currLen {
		diff := currLen - len(it.blockReaders)
		for i := 0; i < diff; i++ {
			seg := ts.NewSegment(nil, nil, 0, ts.FinalizeNone)
			sr := xio.NewSegmentReader(seg)
			br := xio.BlockReader{
				SegmentReader: sr,
				Start:         start,
				BlockSize:     blockSize,
			}
			it.blockReaders = append(it.blockReaders, br)
		}
	}

	// Set the segment readers to reader from current segment pieces
	segment := it.segments[it.idx]
	if segment.Merged != nil {
		it.resetReader(it.blockReaders[0], segment.Merged)
	} else {
		for i := 0; i < currLen; i++ {
			it.resetReader(it.blockReaders[i], segment.Unmerged[i])
		}
	}

	return true
}

func (it *readerSliceOfSlicesIterator) resetReader(
	r xio.BlockReader,
	seg *rpc.Segment,
) {
	rseg, err := r.Segment()
	_, start, end := it.CurrentReaders()

	if err != nil {
		r.ResetWindowed(ts.Segment{}, start, end)
		return
	}

	var (
		head = rseg.Head
		tail = rseg.Tail
	)
	if head == nil {
		head = checked.NewBytes(seg.Head, nil)
		head.IncRef()
	} else {
		head.Reset(seg.Head)
	}
	if tail == nil {
		tail = checked.NewBytes(seg.Tail, nil)
		tail.IncRef()
	} else {
		tail.Reset(seg.Tail)
	}

	var checksum uint32
	if seg.Checksum != nil {
		checksum = uint32(*seg.Checksum)
	}

	newSeg := ts.NewSegment(head, tail, checksum, ts.FinalizeNone)
	r.ResetWindowed(newSeg, start, end)
}

func (it *readerSliceOfSlicesIterator) currentLen() int {
	if it.segments[it.idx].Merged != nil {
		return 1
	}
	return len(it.segments[it.idx].Unmerged)
}

func (it *readerSliceOfSlicesIterator) CurrentReaders() (int, time.Time, time.Duration) {
	segments := it.segments[it.idx]
	if segments.Merged != nil {
		return 1, timeConvert(segments.Merged.StartTime), durationConvert(segments.Merged.BlockSize)
	}
	unmerged := it.currentLen()
	if unmerged == 0 {
		return 0, timeZero, 0
	}
	return unmerged, timeConvert(segments.Unmerged[0].StartTime), durationConvert(segments.Unmerged[0].BlockSize)
}

func timeConvert(ticks *int64) time.Time {
	if ticks == nil {
		return timeZero
	}
	return xtime.FromNormalizedTime(*ticks, time.Nanosecond)
}

func durationConvert(duration *int64) time.Duration {
	if duration == nil {
		return 0
	}
	return xtime.FromNormalizedDuration(*duration, time.Nanosecond)
}

func (it *readerSliceOfSlicesIterator) CurrentReaderAt(idx int) xio.BlockReader {
	if idx >= it.currentLen() {
		return xio.EmptyBlockReader
	}
	return it.blockReaders[idx]
}

func (it *readerSliceOfSlicesIterator) Close() {
	if it.closed {
		return
	}
	it.closed = true
	// Release any refs to segments
	it.segments = nil
	// Release any refs to segment byte slices
	for i := range it.blockReaders {
		seg, err := it.blockReaders[i].Segment()
		if err != nil {
			continue
		}
		if seg.Head != nil {
			seg.Head.Reset(nil)
		}
		if seg.Tail != nil {
			seg.Tail.Reset(nil)
		}
	}
	if pool := it.pool; pool != nil {
		pool.Put(it)
	}
}

func (it *readerSliceOfSlicesIterator) Reset(segments []*rpc.Segments) {
	it.segments = segments
	it.resetIndex()
	it.closed = false
}

func (it *readerSliceOfSlicesIterator) Size() (int, error) {
	size := 0
	for _, reader := range it.blockReaders {
		seg, err := reader.Segment()
		if err != nil {
			return 0, err
		}
		size += seg.Len()
	}
	return size, nil
}

func (it *readerSliceOfSlicesIterator) Rewind() {
	it.resetIndex()
}

func (it *readerSliceOfSlicesIterator) resetIndex() {
	it.idx = -1
}
