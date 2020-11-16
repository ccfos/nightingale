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

package encoding

import (
	"github.com/m3db/m3/src/x/checked"
	xtime "github.com/m3db/m3/src/x/time"
)

const (
	// special markers
	defaultEndOfStreamMarker Marker = iota
	defaultAnnotationMarker
	defaultTimeUnitMarker

	// marker encoding information
	defaultMarkerOpcode        = 0x100
	defaultNumMarkerOpcodeBits = 9
	defaultNumMarkerValueBits  = 2
)

var (
	// default time encoding schemes
	defaultZeroBucket             = newTimeBucket(0x0, 1, 0)
	defaultNumValueBitsForBuckets = []int{7, 9, 12}

	// TODO(xichen): set more reasonable defaults once we have more knowledge
	// of the use cases for time units other than seconds.
	defaultTimeEncodingSchemes = map[xtime.Unit]TimeEncodingScheme{
		xtime.Second:      newTimeEncodingScheme(defaultNumValueBitsForBuckets, 32),
		xtime.Millisecond: newTimeEncodingScheme(defaultNumValueBitsForBuckets, 32),
		xtime.Microsecond: newTimeEncodingScheme(defaultNumValueBitsForBuckets, 64),
		xtime.Nanosecond:  newTimeEncodingScheme(defaultNumValueBitsForBuckets, 64),
	}

	// default marker encoding scheme
	defaultMarkerEncodingScheme = newMarkerEncodingScheme(
		defaultMarkerOpcode,
		defaultNumMarkerOpcodeBits,
		defaultNumMarkerValueBits,
		defaultEndOfStreamMarker,
		defaultAnnotationMarker,
		defaultTimeUnitMarker,
	)
)

// TimeBucket represents a bucket for encoding time values.
type TimeBucket interface {

	// Opcode is the opcode prefix used to encode all time values in this range.
	Opcode() uint64

	// NumOpcodeBits is the number of bits used to write the opcode.
	NumOpcodeBits() int

	// Min is the minimum time value accepted in this range.
	Min() int64

	// Max is the maximum time value accepted in this range.
	Max() int64

	// NumValueBits is the number of bits used to write the time value.
	NumValueBits() int
}

type timeBucket struct {
	min           int64
	max           int64
	opcode        uint64
	numOpcodeBits int
	numValueBits  int
}

// newTimeBucket creates a new time bucket.
func newTimeBucket(opcode uint64, numOpcodeBits, numValueBits int) TimeBucket {
	return &timeBucket{
		opcode:        opcode,
		numOpcodeBits: numOpcodeBits,
		numValueBits:  numValueBits,
		min:           -(1 << uint(numValueBits-1)),
		max:           (1 << uint(numValueBits-1)) - 1,
	}
}

func (tb *timeBucket) Opcode() uint64     { return tb.opcode }
func (tb *timeBucket) NumOpcodeBits() int { return tb.numOpcodeBits }
func (tb *timeBucket) Min() int64         { return tb.min }
func (tb *timeBucket) Max() int64         { return tb.max }
func (tb *timeBucket) NumValueBits() int  { return tb.numValueBits }

// TimeEncodingScheme captures information related to time encoding.
type TimeEncodingScheme interface {

	// ZeroBucket is time bucket for encoding zero time values.
	ZeroBucket() TimeBucket

	// Buckets are the ordered time buckets used to encode non-zero, non-default time values.
	Buckets() []TimeBucket

	// DefaultBucket is the time bucket for catching all other time values not included in the regular buckets.
	DefaultBucket() TimeBucket
}

type timeEncodingScheme struct {
	zeroBucket    TimeBucket
	buckets       []TimeBucket
	defaultBucket TimeBucket
}

// newTimeEncodingSchemes converts the unit-to-scheme mapping
// to the underlying TimeEncodingSchemes used for lookups.
func newTimeEncodingSchemes(schemes map[xtime.Unit]TimeEncodingScheme) TimeEncodingSchemes {
	encodingSchemes := make(TimeEncodingSchemes, xtime.UnitCount())
	for k, v := range schemes {
		if !k.IsValid() {
			continue
		}

		encodingSchemes[k] = v
	}

	return encodingSchemes
}

// newTimeEncodingScheme creates a new time encoding scheme.
// NB(xichen): numValueBitsForBbuckets should be ordered by value in ascending order (smallest value first).
func newTimeEncodingScheme(numValueBitsForBuckets []int, numValueBitsForDefault int) TimeEncodingScheme {
	numBuckets := len(numValueBitsForBuckets)
	buckets := make([]TimeBucket, 0, numBuckets)
	numOpcodeBits := 1
	opcode := uint64(0)
	i := 0
	for i < numBuckets {
		opcode = uint64(1<<uint(i+1)) | opcode
		buckets = append(buckets, newTimeBucket(opcode, numOpcodeBits+1, numValueBitsForBuckets[i]))
		i++
		numOpcodeBits++
	}
	defaultBucket := newTimeBucket(opcode|0x1, numOpcodeBits, numValueBitsForDefault)

	return &timeEncodingScheme{
		zeroBucket:    defaultZeroBucket,
		buckets:       buckets,
		defaultBucket: defaultBucket,
	}
}

func (tes *timeEncodingScheme) ZeroBucket() TimeBucket    { return tes.zeroBucket }
func (tes *timeEncodingScheme) Buckets() []TimeBucket     { return tes.buckets }
func (tes *timeEncodingScheme) DefaultBucket() TimeBucket { return tes.defaultBucket }

// TimeEncodingSchemes defines the time encoding schemes for different time units.
type TimeEncodingSchemes []TimeEncodingScheme

// SchemeForUnit returns the corresponding TimeEncodingScheme for the provided unit.
// Returns false if the unit does not match a scheme or is invalid.
func (s TimeEncodingSchemes) SchemeForUnit(u xtime.Unit) (TimeEncodingScheme, bool) {
	if !u.IsValid() || int(u) >= len(s) {
		return nil, false
	}

	scheme := s[u]
	if scheme == nil {
		return nil, false
	}

	return s[u], true
}

// Marker represents the markers.
type Marker byte

// MarkerEncodingScheme captures the information related to marker encoding.
type MarkerEncodingScheme interface {

	// Opcode returns the marker opcode.
	Opcode() uint64

	// NumOpcodeBits returns the number of bits used for the opcode.
	NumOpcodeBits() int

	// NumValueBits returns the number of bits used for the marker value.
	NumValueBits() int

	// EndOfStream returns the end of stream marker.
	EndOfStream() Marker

	// Annotation returns the annotation marker.
	Annotation() Marker

	// TimeUnit returns the time unit marker.
	TimeUnit() Marker

	// Tail will return the tail portion of a stream including the relevant bits
	// in the last byte along with the end of stream marker.
	Tail(streamLastByte byte, streamCurrentPosition int) checked.Bytes
}

type markerEncodingScheme struct {
	opcode        uint64
	numOpcodeBits int
	numValueBits  int
	endOfStream   Marker
	annotation    Marker
	timeUnit      Marker
	tails         [256][8]checked.Bytes
}

func newMarkerEncodingScheme(
	opcode uint64,
	numOpcodeBits int,
	numValueBits int,
	endOfStream Marker,
	annotation Marker,
	timeUnit Marker,
) MarkerEncodingScheme {
	scheme := &markerEncodingScheme{
		opcode:        opcode,
		numOpcodeBits: numOpcodeBits,
		numValueBits:  numValueBits,
		endOfStream:   endOfStream,
		annotation:    annotation,
		timeUnit:      timeUnit,
	}
	// NB(r): we precompute all possible tail streams dependent on last byte
	// so we never have to pool or allocate tails for each stream when we
	// want to take a snapshot of the current stream returned by the `Stream` method.
	for i := range scheme.tails {
		for j := range scheme.tails[i] {
			pos := j + 1
			tmp := NewOStream(checked.NewBytes(nil, nil), false, nil)
			tmp.WriteBits(uint64(i)>>uint(8-pos), pos)
			WriteSpecialMarker(tmp, scheme, endOfStream)
			rawBytes, _ := tmp.RawBytes()
			tail := checked.NewBytes(rawBytes, nil)
			scheme.tails[i][j] = tail
		}
	}
	return scheme
}

// WriteSpecialMarker writes the marker that marks the start of a special symbol,
// e.g., the eos marker, the annotation marker, or the time unit marker.
func WriteSpecialMarker(os OStream, scheme MarkerEncodingScheme, marker Marker) {
	os.WriteBits(scheme.Opcode(), scheme.NumOpcodeBits())
	os.WriteBits(uint64(marker), scheme.NumValueBits())
}

func (mes *markerEncodingScheme) Opcode() uint64                     { return mes.opcode }
func (mes *markerEncodingScheme) NumOpcodeBits() int                 { return mes.numOpcodeBits }
func (mes *markerEncodingScheme) NumValueBits() int                  { return mes.numValueBits }
func (mes *markerEncodingScheme) EndOfStream() Marker                { return mes.endOfStream }
func (mes *markerEncodingScheme) Annotation() Marker                 { return mes.annotation }
func (mes *markerEncodingScheme) TimeUnit() Marker                   { return mes.timeUnit }
func (mes *markerEncodingScheme) Tail(b byte, pos int) checked.Bytes { return mes.tails[int(b)][pos-1] }
