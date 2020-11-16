// Copyright (c) 2019 Uber Technologies, Inc.
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
	"encoding/binary"
	"fmt"
	"time"

	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/ts"
	xtime "github.com/m3db/m3/src/x/time"
)

// TimestampIterator encapsulates all the state required for iterating over
// delta-of-delta compresed timestamps.
type TimestampIterator struct {
	PrevTime      xtime.UnixNano
	PrevTimeDelta time.Duration
	PrevAnt       ts.Annotation

	TimeUnit xtime.Unit

	Opts encoding.Options

	TimeUnitChanged bool
	Done            bool

	// Controls whether the iterator will "look ahead" for marker encoding
	// schemes. Setting SkipMarkers to true disables the look ahead behavior
	// for situations where looking ahead is not safe.
	SkipMarkers bool

	numValueBits         uint
	numBits              uint
	markerEncodingScheme encoding.MarkerEncodingScheme
}

// NewTimestampIterator creates a new TimestampIterator.
func NewTimestampIterator(opts encoding.Options, skipMarkers bool) TimestampIterator {
	mes := opts.MarkerEncodingScheme()
	return TimestampIterator{
		Opts:                 opts,
		SkipMarkers:          skipMarkers,
		numValueBits:         uint(mes.NumValueBits()),
		numBits:              uint(mes.NumOpcodeBits() + mes.NumValueBits()),
		markerEncodingScheme: mes,
	}
}

// ReadTimestamp reads the first or next timestamp.
func (it *TimestampIterator) ReadTimestamp(stream encoding.IStream) (bool, bool, error) {
	it.PrevAnt = nil

	var (
		first = false
		err   error
	)
	if it.PrevTime == 0 {
		first = true
		err = it.readFirstTimestamp(stream)
	} else {
		err = it.readNextTimestamp(stream)
	}
	if err != nil {
		return false, false, err
	}

	// NB(xichen): reset time delta to 0 when there is a time unit change to be
	// consistent with the encoder.
	if it.TimeUnitChanged {
		it.PrevTimeDelta = 0
		it.TimeUnitChanged = false
	}

	return first, it.Done, nil
}

// ReadTimeUnit reads an encoded time unit and updates the iterator's state
// accordingly. It is exposed as a public method so that callers can control
// the encoding / decoding of the time unit on their own if they choose.
func (it *TimestampIterator) ReadTimeUnit(stream encoding.IStream) error {
	tuBits, err := stream.ReadByte()
	if err != nil {
		return err
	}

	tu := xtime.Unit(tuBits)
	if tu.IsValid() && tu != it.TimeUnit {
		it.TimeUnitChanged = true
	}
	it.TimeUnit = tu

	return nil
}

func (it *TimestampIterator) readFirstTimestamp(stream encoding.IStream) error {
	ntBits, err := stream.ReadBits(64)
	if err != nil {
		return err
	}

	// NB(xichen): first time stamp is always normalized to nanoseconds.
	nt := xtime.UnixNano(ntBits)
	if it.TimeUnit == xtime.None {
		it.TimeUnit = initialTimeUnit(nt, it.Opts.DefaultTimeUnit())
	}

	err = it.readNextTimestamp(stream)
	if err != nil {
		return err
	}

	it.PrevTime = nt + xtime.UnixNano(it.PrevTimeDelta)
	return nil
}

func (it *TimestampIterator) readNextTimestamp(stream encoding.IStream) error {
	dod, err := it.readMarkerOrDeltaOfDelta(stream)
	if err != nil {
		return err
	}

	it.PrevTimeDelta += dod
	it.PrevTime = it.PrevTime + xtime.UnixNano(it.PrevTimeDelta)
	return nil
}

func (it *TimestampIterator) tryReadMarker(stream encoding.IStream) (time.Duration, bool, error) {
	opcodeAndValue, success := it.tryPeekBits(stream, it.numBits)
	if !success {
		return 0, false, nil
	}

	opcode := opcodeAndValue >> it.numValueBits
	if opcode != it.markerEncodingScheme.Opcode() {
		return 0, false, nil
	}

	var (
		valueMask   = (1 << it.numValueBits) - 1
		markerValue = int64(opcodeAndValue & uint64(valueMask))
	)
	switch encoding.Marker(markerValue) {
	case it.markerEncodingScheme.EndOfStream():
		_, err := stream.ReadBits(it.numBits)
		if err != nil {
			return 0, false, err
		}
		it.Done = true
		return 0, true, nil
	case it.markerEncodingScheme.Annotation():
		_, err := stream.ReadBits(it.numBits)
		if err != nil {
			return 0, false, err
		}
		err = it.readAnnotation(stream)
		if err != nil {
			return 0, false, err
		}
		markerOrDOD, err := it.readMarkerOrDeltaOfDelta(stream)
		if err != nil {
			return 0, false, err
		}
		return markerOrDOD, true, nil
	case it.markerEncodingScheme.TimeUnit():
		_, err := stream.ReadBits(it.numBits)
		if err != nil {
			return 0, false, err
		}
		err = it.ReadTimeUnit(stream)
		if err != nil {
			return 0, false, err
		}
		markerOrDOD, err := it.readMarkerOrDeltaOfDelta(stream)
		if err != nil {
			return 0, false, err
		}
		return markerOrDOD, true, nil
	default:
		return 0, false, nil
	}
}

func (it *TimestampIterator) readMarkerOrDeltaOfDelta(stream encoding.IStream) (time.Duration, error) {
	if !it.SkipMarkers {
		dod, success, err := it.tryReadMarker(stream)
		if err != nil {
			return 0, err
		}
		if it.Done {
			return 0, nil
		}

		if success {
			return dod, nil
		}
	}

	tes, exists := it.Opts.TimeEncodingSchemes().SchemeForUnit(it.TimeUnit)
	if !exists {
		return 0, fmt.Errorf("time encoding scheme for time unit %v doesn't exist", it.TimeUnit)
	}

	return it.readDeltaOfDelta(stream, tes)
}

func (it *TimestampIterator) readDeltaOfDelta(
	stream encoding.IStream, tes encoding.TimeEncodingScheme) (time.Duration, error) {
	if it.TimeUnitChanged {
		// NB(xichen): if the time unit has changed, always read 64 bits as normalized
		// dod in nanoseconds.
		dodBits, err := stream.ReadBits(64)
		if err != nil {
			return 0, err
		}

		dod := encoding.SignExtend(dodBits, 64)
		return time.Duration(dod), nil
	}

	cb, err := stream.ReadBits(1)
	if err != nil {
		return 0, err
	}
	if cb == tes.ZeroBucket().Opcode() {
		return 0, nil
	}

	buckets := tes.Buckets()
	for i := 0; i < len(buckets); i++ {
		nextCB, err := stream.ReadBits(1)
		if err != nil {
			return 0, nil
		}

		cb = (cb << 1) | nextCB
		if cb == buckets[i].Opcode() {
			dodBits, err := stream.ReadBits(uint(buckets[i].NumValueBits()))
			if err != nil {
				return 0, err
			}

			dod := encoding.SignExtend(dodBits, uint(buckets[i].NumValueBits()))
			timeUnit, err := it.TimeUnit.Value()
			if err != nil {
				return 0, nil
			}

			return xtime.FromNormalizedDuration(dod, timeUnit), nil
		}
	}

	numValueBits := uint(tes.DefaultBucket().NumValueBits())
	dodBits, err := stream.ReadBits(numValueBits)
	if err != nil {
		return 0, err
	}
	dod := encoding.SignExtend(dodBits, numValueBits)
	timeUnit, err := it.TimeUnit.Value()
	if err != nil {
		return 0, nil
	}

	return xtime.FromNormalizedDuration(dod, timeUnit), nil
}

func (it *TimestampIterator) readAnnotation(stream encoding.IStream) error {
	antLen, err := it.readVarint(stream)
	if err != nil {
		return err
	}

	// NB: we add 1 here to offset the 1 we subtracted during encoding.
	antLen = antLen + 1
	if antLen <= 0 {
		return fmt.Errorf("unexpected annotation length %d", antLen)
	}

	// TODO(xichen): use pool to allocate the buffer once the pool diff lands.
	buf := make([]byte, antLen)
	n, err := stream.Read(buf)
	if err != nil {
		return err
	}
	if n != antLen {
		return fmt.Errorf(
			"expected to read %d annotation bytes but read: %d",
			antLen, n)
	}
	it.PrevAnt = buf

	return nil
}

func (it *TimestampIterator) readVarint(stream encoding.IStream) (int, error) {
	res, err := binary.ReadVarint(stream)
	return int(res), err
}

func (it *TimestampIterator) tryPeekBits(stream encoding.IStream, numBits uint) (uint64, bool) {
	res, err := stream.PeekBits(numBits)
	if err != nil {
		return 0, false
	}
	return res, true
}
