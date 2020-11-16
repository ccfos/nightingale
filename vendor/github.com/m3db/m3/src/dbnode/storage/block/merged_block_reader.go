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

package block

import (
	"fmt"
	"sync"
	"time"

	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/namespace"
	"github.com/m3db/m3/src/dbnode/ts"
	"github.com/m3db/m3/src/dbnode/x/xio"
	"github.com/m3db/m3/src/x/context"
	"github.com/m3db/m3/src/x/pool"
)

type dbMergedBlockReader struct {
	sync.RWMutex
	ctx        context.Context
	opts       Options
	blockStart time.Time
	blockSize  time.Duration
	streams    [2]mergeableStream
	readers    [2]xio.SegmentReader
	merged     xio.BlockReader
	encoder    encoding.Encoder
	err        error
	nsCtx      namespace.Context
}

type mergeableStream struct {
	stream   xio.SegmentReader
	finalize bool
}

func (ms mergeableStream) clone(pool pool.CheckedBytesPool) (mergeableStream, error) {
	stream, err := ms.stream.Clone(pool)
	if err != nil {
		return mergeableStream{}, err
	}
	return mergeableStream{
		stream:   stream,
		finalize: ms.finalize,
	}, nil
}

func newDatabaseMergedBlockReader(
	nsCtx namespace.Context,
	blockStart time.Time,
	blockSize time.Duration,
	streamA, streamB mergeableStream,
	opts Options,
) xio.BlockReader {
	r := &dbMergedBlockReader{
		ctx:        opts.ContextPool().Get(),
		nsCtx:      nsCtx,
		opts:       opts,
		blockStart: blockStart,
		blockSize:  blockSize,
	}
	r.streams[0] = streamA
	r.streams[1] = streamB
	r.readers[0] = streamA.stream
	r.readers[1] = streamB.stream
	return xio.BlockReader{
		SegmentReader: r,
		Start:         blockStart,
		BlockSize:     blockSize,
	}
}

func (r *dbMergedBlockReader) mergedReader() (xio.BlockReader, error) {
	r.RLock()
	if r.merged.IsNotEmpty() || r.err != nil {
		r.RUnlock()
		return r.merged, r.err
	}
	r.RUnlock()

	r.Lock()
	defer r.Unlock()

	if r.merged.IsNotEmpty() || r.err != nil {
		return r.merged, r.err
	}

	multiIter := r.opts.MultiReaderIteratorPool().Get()
	multiIter.Reset(r.readers[:], r.blockStart, r.blockSize, r.nsCtx.Schema)
	defer multiIter.Close()

	r.encoder = r.opts.EncoderPool().Get()
	r.encoder.Reset(r.blockStart, r.opts.DatabaseBlockAllocSize(), r.nsCtx.Schema)

	for multiIter.Next() {
		dp, unit, annotation := multiIter.Current()
		err := r.encoder.Encode(dp, unit, annotation)
		if err != nil {
			r.encoder.Close()
			r.err = err
			return xio.EmptyBlockReader, err
		}
	}
	if err := multiIter.Err(); err != nil {
		r.encoder.Close()
		r.err = err
		return xio.EmptyBlockReader, err
	}

	// Release references to the existing streams
	for i := range r.streams {
		if r.streams[i].stream != nil && r.streams[i].finalize {
			r.streams[i].stream.Finalize()
		}
		r.streams[i].stream = nil
	}
	for i := range r.readers {
		r.readers[i] = nil
	}

	// Can ignore OK here because BlockReader will handle nil streams
	// properly.
	stream, _ := r.encoder.Stream(r.ctx)
	r.merged = xio.BlockReader{
		SegmentReader: stream,
		Start:         r.blockStart,
		BlockSize:     r.blockSize,
	}

	return r.merged, nil
}

func (r *dbMergedBlockReader) Clone(
	pool pool.CheckedBytesPool,
) (xio.SegmentReader, error) {
	s0, err := r.streams[0].clone(pool)
	if err != nil {
		return nil, err
	}
	s1, err := r.streams[1].clone(pool)
	if err != nil {
		return nil, err
	}
	return newDatabaseMergedBlockReader(
		r.nsCtx,
		r.blockStart,
		r.blockSize,
		s0,
		s1,
		r.opts,
	), nil
}

func (r *dbMergedBlockReader) Start() time.Time {
	return r.blockStart
}

func (r *dbMergedBlockReader) BlockSize() time.Duration {
	return r.blockSize
}

func (r *dbMergedBlockReader) Read(b []byte) (int, error) {
	reader, err := r.mergedReader()
	if err != nil {
		return 0, err
	}
	return reader.Read(b)
}

func (r *dbMergedBlockReader) Segment() (ts.Segment, error) {
	reader, err := r.mergedReader()
	if err != nil {
		return ts.Segment{}, err
	}
	return reader.Segment()
}

func (r *dbMergedBlockReader) SegmentReader() (xio.SegmentReader, error) {
	reader, err := r.mergedReader()
	if err != nil {
		return nil, err
	}
	return reader.SegmentReader, nil
}

func (r *dbMergedBlockReader) Reset(_ ts.Segment) {
	panic(fmt.Errorf("merged block reader not available for re-use"))
}

func (r *dbMergedBlockReader) ResetWindowed(_ ts.Segment, _, _ time.Time) {
	panic(fmt.Errorf("merged block reader not available for re-use"))
}

func (r *dbMergedBlockReader) Finalize() {
	r.Lock()

	// Can blocking close, the finalizer will complete immediately
	// since it just dec refs on the buffer it created in the encoder.
	r.ctx.BlockingClose()

	r.blockStart = time.Time{}

	for i := range r.streams {
		if r.streams[i].stream != nil && r.streams[i].finalize {
			r.streams[i].stream.Finalize()
		}
		r.streams[i].stream = nil
	}
	for i := range r.readers {
		if r.readers[i] != nil {
			r.readers[i] = nil
		}
	}

	if r.merged.IsNotEmpty() {
		r.merged.Finalize()
	}
	r.merged = xio.EmptyBlockReader

	if r.encoder != nil {
		r.encoder.Close()
	}
	r.encoder = nil

	r.err = nil

	r.Unlock()
}
