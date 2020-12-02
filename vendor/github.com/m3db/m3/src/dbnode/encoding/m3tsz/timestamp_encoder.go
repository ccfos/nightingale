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
	"bytes"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/ts"
	xtime "github.com/m3db/m3/src/x/time"
)

// TimestampEncoder encapsulates the state required for a logical stream of
// bits that represent a stream of timestamps compressed using delta-of-delta
type TimestampEncoder struct {
	PrevTime       time.Time
	PrevTimeDelta  time.Duration
	PrevAnnotation []byte

	Options encoding.Options

	TimeUnit xtime.Unit

	// Used to keep track of time unit changes that occur directly via the WriteTimeUnit()
	// API as opposed to indirectly via the WriteTime() API.
	timeUnitEncodedManually bool
	// Only taken into account if using the WriteTime() API.
	hasWrittenFirst bool
}

// NewTimestampEncoder creates a new TimestampEncoder.
func NewTimestampEncoder(
	start time.Time, timeUnit xtime.Unit, opts encoding.Options) TimestampEncoder {
	return TimestampEncoder{
		PrevTime: start,
		TimeUnit: initialTimeUnit(xtime.ToUnixNano(start), timeUnit),
		Options:  opts,
	}
}

// WriteTime encode the timestamp using delta-of-delta compression.
func (enc *TimestampEncoder) WriteTime(
	stream encoding.OStream, currTime time.Time, ant ts.Annotation, timeUnit xtime.Unit) error {
	if !enc.hasWrittenFirst {
		if err := enc.WriteFirstTime(stream, currTime, ant, timeUnit); err != nil {
			return err
		}
		enc.hasWrittenFirst = true
		return nil
	}

	return enc.WriteNextTime(stream, currTime, ant, timeUnit)
}

// WriteFirstTime encodes the first timestamp.
func (enc *TimestampEncoder) WriteFirstTime(
	stream encoding.OStream, currTime time.Time, ant ts.Annotation, timeUnit xtime.Unit) error {
	// NB(xichen): Always write the first time in nanoseconds because we don't know
	// if the start time is going to be a multiple of the time unit provided.
	nt := xtime.ToNormalizedTime(enc.PrevTime, time.Nanosecond)
	stream.WriteBits(uint64(nt), 64)
	return enc.WriteNextTime(stream, currTime, ant, timeUnit)
}

// WriteNextTime encodes the next (non-first) timestamp.
func (enc *TimestampEncoder) WriteNextTime(
	stream encoding.OStream, currTime time.Time, ant ts.Annotation, timeUnit xtime.Unit) error {
	enc.writeAnnotation(stream, ant)
	tuChanged := enc.maybeWriteTimeUnitChange(stream, timeUnit)

	timeDelta := currTime.Sub(enc.PrevTime)
	enc.PrevTime = currTime
	if tuChanged || enc.timeUnitEncodedManually {
		enc.writeDeltaOfDeltaTimeUnitChanged(stream, enc.PrevTimeDelta, timeDelta)
		// NB(xichen): if the time unit has changed, we reset the time delta to zero
		// because we can't guarantee that dt is a multiple of the new time unit, which
		// means we can't guarantee that the delta of delta when encoding the next
		// data point is a multiple of the new time unit.
		enc.PrevTimeDelta = 0
		enc.timeUnitEncodedManually = false
		return nil
	}
	err := enc.writeDeltaOfDeltaTimeUnitUnchanged(
		stream, enc.PrevTimeDelta, timeDelta, timeUnit)
	enc.PrevTimeDelta = timeDelta
	return err
}

// WriteTimeUnit writes the new time unit into the stream. It exists as a standalone method
// so that other calls can encode time unit changes without relying on the marker scheme.
func (enc *TimestampEncoder) WriteTimeUnit(stream encoding.OStream, timeUnit xtime.Unit) {
	stream.WriteByte(byte(timeUnit))
	enc.TimeUnit = timeUnit
	enc.timeUnitEncodedManually = true
}

// maybeWriteTimeUnitChange encodes the time unit and returns true if the time unit has
// changed, and false otherwise.
func (enc *TimestampEncoder) maybeWriteTimeUnitChange(stream encoding.OStream, timeUnit xtime.Unit) bool {
	if !enc.shouldWriteTimeUnit(timeUnit) {
		return false
	}

	scheme := enc.Options.MarkerEncodingScheme()
	encoding.WriteSpecialMarker(stream, scheme, scheme.TimeUnit())
	enc.WriteTimeUnit(stream, timeUnit)
	return true
}

// shouldWriteTimeUnit determines whether we should write tu as a time unit.
// Returns true if tu is valid and differs from the existing time unit, false otherwise.
func (enc *TimestampEncoder) shouldWriteTimeUnit(timeUnit xtime.Unit) bool {
	if !timeUnit.IsValid() || timeUnit == enc.TimeUnit {
		return false
	}
	return true
}

// shouldWriteAnnotation determines whether we should write ant as an annotation.
// Returns true if ant is not empty and differs from the existing annotation, false otherwise.
func (enc *TimestampEncoder) shouldWriteAnnotation(ant ts.Annotation) bool {
	numAnnotationBytes := len(ant)
	if numAnnotationBytes == 0 {
		return false
	}
	return !bytes.Equal(enc.PrevAnnotation, ant)
}

func (enc *TimestampEncoder) writeAnnotation(stream encoding.OStream, ant ts.Annotation) {
	if !enc.shouldWriteAnnotation(ant) {
		return
	}

	scheme := enc.Options.MarkerEncodingScheme()
	encoding.WriteSpecialMarker(stream, scheme, scheme.Annotation())

	var buf [binary.MaxVarintLen32]byte
	// NB: we subtract 1 for possible varint encoding savings
	annotationLength := binary.PutVarint(buf[:], int64(len(ant)-1))

	stream.WriteBytes(buf[:annotationLength])
	stream.WriteBytes(ant)
	enc.PrevAnnotation = ant
}

func (enc *TimestampEncoder) writeDeltaOfDeltaTimeUnitChanged(
	stream encoding.OStream, prevDelta, curDelta time.Duration) {
	// NB(xichen): if the time unit has changed, always normalize delta-of-delta
	// to nanoseconds and encode it using 64 bits.
	dodInNano := int64(curDelta - prevDelta)
	stream.WriteBits(uint64(dodInNano), 64)
}

func (enc *TimestampEncoder) writeDeltaOfDeltaTimeUnitUnchanged(
	stream encoding.OStream, prevDelta, curDelta time.Duration, timeUnit xtime.Unit) error {
	u, err := timeUnit.Value()
	if err != nil {
		return err
	}

	deltaOfDelta := xtime.ToNormalizedDuration(curDelta-prevDelta, u)
	tes, exists := enc.Options.TimeEncodingSchemes().SchemeForUnit(timeUnit)
	if !exists {
		return fmt.Errorf("time encoding scheme for time unit %v doesn't exist", timeUnit)
	}

	if deltaOfDelta == 0 {
		zeroBucket := tes.ZeroBucket()
		stream.WriteBits(zeroBucket.Opcode(), zeroBucket.NumOpcodeBits())
		return nil
	}

	buckets := tes.Buckets()
	for i := 0; i < len(buckets); i++ {
		if deltaOfDelta >= buckets[i].Min() && deltaOfDelta <= buckets[i].Max() {
			stream.WriteBits(buckets[i].Opcode(), buckets[i].NumOpcodeBits())
			stream.WriteBits(uint64(deltaOfDelta), buckets[i].NumValueBits())
			return nil
		}
	}
	defaultBucket := tes.DefaultBucket()
	stream.WriteBits(defaultBucket.Opcode(), defaultBucket.NumOpcodeBits())
	stream.WriteBits(uint64(deltaOfDelta), defaultBucket.NumValueBits())
	return nil
}

func initialTimeUnit(start xtime.UnixNano, tu xtime.Unit) xtime.Unit {
	tv, err := tu.Value()
	if err != nil {
		return xtime.None
	}
	// If we want to use tu as the time unit for start, start must
	// be a multiple of tu.
	if start%xtime.UnixNano(tv) == 0 {
		return tu
	}
	return xtime.None
}
