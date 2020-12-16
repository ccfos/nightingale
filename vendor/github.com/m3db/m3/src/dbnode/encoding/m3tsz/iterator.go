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

package m3tsz

import (
	"io"
	"math"

	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/namespace"
	"github.com/m3db/m3/src/dbnode/ts"
	xtime "github.com/m3db/m3/src/x/time"
)

// readerIterator provides an interface for clients to incrementally
// read datapoints off of an encoded stream.
type readerIterator struct {
	is   encoding.IStream
	opts encoding.Options

	err        error   // current error
	intVal     float64 // current int value
	tsIterator TimestampIterator
	floatIter  FloatEncoderAndIterator

	mult uint8 // current int multiplier
	sig  uint8 // current number of significant bits for int diff

	intOptimized bool // whether encoding scheme is optimized for ints
	isFloat      bool // whether encoding is in int or float

	closed bool
}

// NewReaderIterator returns a new iterator for a given reader
func NewReaderIterator(reader io.Reader, intOptimized bool, opts encoding.Options) encoding.ReaderIterator {
	return &readerIterator{
		is:           encoding.NewIStream(reader, opts.IStreamReaderSizeM3TSZ()),
		opts:         opts,
		tsIterator:   NewTimestampIterator(opts, false),
		intOptimized: intOptimized,
	}
}

// Next moves to the next item
func (it *readerIterator) Next() bool {
	if !it.hasNext() {
		return false
	}

	first, done, err := it.tsIterator.ReadTimestamp(it.is)
	if err != nil || done {
		it.err = err
		return false
	}

	it.readValue(first)

	return it.hasNext()
}

func (it *readerIterator) readValue(first bool) {
	if first {
		it.readFirstValue()
	} else {
		it.readNextValue()
	}
}

func (it *readerIterator) readFirstValue() {
	if !it.intOptimized {
		if err := it.floatIter.readFullFloat(it.is); err != nil {
			it.err = err
		}
		return
	}

	if it.readBits(1) == opcodeFloatMode {
		if err := it.floatIter.readFullFloat(it.is); err != nil {
			it.err = err
		}
		it.isFloat = true
		return
	}

	it.readIntSigMult()
	it.readIntValDiff()
}

func (it *readerIterator) readNextValue() {
	if !it.intOptimized {
		if err := it.floatIter.readNextFloat(it.is); err != nil {
			it.err = err
		}
		return
	}

	if it.readBits(1) == opcodeUpdate {
		if it.readBits(1) == opcodeRepeat {
			return
		}

		if it.readBits(1) == opcodeFloatMode {
			// Change to floatVal
			if err := it.floatIter.readFullFloat(it.is); err != nil {
				it.err = err
			}
			it.isFloat = true
			return
		}

		it.readIntSigMult()
		it.readIntValDiff()
		it.isFloat = false
		return
	}

	if it.isFloat {
		if err := it.floatIter.readNextFloat(it.is); err != nil {
			it.err = err
		}
	} else {
		it.readIntValDiff()
	}
}

func (it *readerIterator) readIntSigMult() {
	if it.readBits(1) == opcodeUpdateSig {
		if it.readBits(1) == OpcodeZeroSig {
			it.sig = 0
		} else {
			it.sig = uint8(it.readBits(NumSigBits)) + 1
		}
	}

	if it.readBits(1) == opcodeUpdateMult {
		it.mult = uint8(it.readBits(numMultBits))
		if it.mult > maxMult {
			it.err = errInvalidMultiplier
		}
	}
}

func (it *readerIterator) readIntValDiff() {
	sign := -1.0
	if it.readBits(1) == opcodeNegative {
		sign = 1.0
	}

	it.intVal += sign * float64(it.readBits(uint(it.sig)))
}

func (it *readerIterator) readBits(numBits uint) uint64 {
	if !it.hasNext() {
		return 0
	}
	var res uint64
	res, it.err = it.is.ReadBits(numBits)
	return res
}

// Current returns the value as well as the annotation associated with the current datapoint.
// Users should not hold on to the returned Annotation object as it may get invalidated when
// the iterator calls Next().
func (it *readerIterator) Current() (ts.Datapoint, xtime.Unit, ts.Annotation) {
	if !it.intOptimized || it.isFloat {
		return ts.Datapoint{
			Timestamp:      it.tsIterator.PrevTime.ToTime(),
			TimestampNanos: it.tsIterator.PrevTime,
			Value:          math.Float64frombits(it.floatIter.PrevFloatBits),
		}, it.tsIterator.TimeUnit, it.tsIterator.PrevAnt
	}

	return ts.Datapoint{
		Timestamp:      it.tsIterator.PrevTime.ToTime(),
		TimestampNanos: it.tsIterator.PrevTime,
		Value:          convertFromIntFloat(it.intVal, it.mult),
	}, it.tsIterator.TimeUnit, it.tsIterator.PrevAnt
}

// Err returns the error encountered
func (it *readerIterator) Err() error {
	return it.err
}

func (it *readerIterator) hasError() bool {
	return it.err != nil
}

func (it *readerIterator) isDone() bool {
	return it.tsIterator.Done
}

func (it *readerIterator) isClosed() bool {
	return it.closed
}

func (it *readerIterator) hasNext() bool {
	return !it.hasError() && !it.isDone() && !it.isClosed()
}

// Reset resets the ReadIterator for reuse.
func (it *readerIterator) Reset(reader io.Reader, schema namespace.SchemaDescr) {
	it.is.Reset(reader)
	it.tsIterator = NewTimestampIterator(it.opts, it.tsIterator.SkipMarkers)
	it.err = nil
	it.isFloat = false
	it.intVal = 0.0
	it.mult = 0
	it.sig = 0
	it.closed = false
}

// Close closes the ReaderIterator.
func (it *readerIterator) Close() {
	if it.closed {
		return
	}

	it.closed = true
	pool := it.opts.ReaderIteratorPool()
	if pool != nil {
		pool.Put(it)
	}
}
