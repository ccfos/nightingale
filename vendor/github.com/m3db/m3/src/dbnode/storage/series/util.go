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

package series

import (
	"time"

	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/x/xio"
	xtime "github.com/m3db/m3/src/x/time"

	"github.com/m3db/m3/src/dbnode/namespace"
)

// ValuesByTime is a sortable slice of DecodedTestValue.
type ValuesByTime []DecodedTestValue

// Len is the number of elements in the collection.
func (v ValuesByTime) Len() int {
	return len(v)
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (v ValuesByTime) Less(lhs, rhs int) bool {
	l := v[lhs].Timestamp
	r := v[rhs].Timestamp
	if l.Equal(r) {
		return v[lhs].Value-v[rhs].Value < 0
	}

	return l.Before(r)
}

// Swap swaps the elements with indexes i and j.
func (v ValuesByTime) Swap(lhs, rhs int) {
	v[lhs], v[rhs] = v[rhs], v[lhs]
}

// DecodedTestValue is a decoded datapoint.
type DecodedTestValue struct {
	// Timestamp is the data point timestamp.
	Timestamp time.Time
	// Value is the data point value.
	Value float64
	// Unit is the data point unit.
	Unit xtime.Unit
	// Annotation is the data point annotation.
	Annotation []byte
}

// DecodeSegmentValues is a test utility to read through a slice of
// SegmentReaders.
func DecodeSegmentValues(
	results []xio.SegmentReader,
	iter encoding.MultiReaderIterator,
	schema namespace.SchemaDescr,
) ([]DecodedTestValue, error) {
	iter.Reset(results, time.Time{}, time.Duration(0), schema)
	defer iter.Close()

	var all []DecodedTestValue
	for iter.Next() {
		dp, unit, annotation := iter.Current()
		// Iterator reuse annotation byte slices, so make a copy.
		annotationCopy := append([]byte(nil), annotation...)
		all = append(all, DecodedTestValue{
			dp.Timestamp, dp.Value, unit, annotationCopy})
	}

	if err := iter.Err(); err != nil {
		return nil, err
	}

	return all, nil
}
