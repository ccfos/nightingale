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

package index

import (
	"github.com/m3db/m3/src/x/ident"
)

// AggregateValues is a collection of unique identity values backed by a pool.
// NB: there are no synchronization guarantees provided by default.
type AggregateValues struct {
	hasValues bool
	valuesMap *AggregateValuesMap
	pool      AggregateValuesPool
}

// NewAggregateValues returns a new AggregateValues object.
func NewAggregateValues(opts Options) AggregateValues {
	return AggregateValues{
		hasValues: true,
		valuesMap: NewAggregateValuesMap(opts.IdentifierPool()),
		pool:      opts.AggregateValuesPool(),
	}
}

// MustNewAggregateValues returns a new AggregateValues object with provided
// idents added in.
func MustNewAggregateValues(opts Options, ids ...ident.ID) AggregateValues {
	m := NewAggregateValues(opts)
	for _, id := range ids {
		if err := m.addValue(id); err != nil {
			panic(err.Error())
		}
	}
	return m
}

// HasValues returns true if this has an aggregate values map.
func (v *AggregateValues) HasValues() bool {
	return v.hasValues
}

// Map returns a map from an ID -> empty struct to signify existence of the
// ID in the set this structure represents.
func (v *AggregateValues) Map() *AggregateValuesMap {
	return v.valuesMap
}

// Size returns the number of IDs tracked.
func (v *AggregateValues) Size() int {
	if !v.hasValues {
		return 0
	}

	return v.valuesMap.Len()
}

func (v *AggregateValues) finalize() {
	// NB: if this aggregate values has no values, no need to finalize.
	if !v.hasValues {
		return
	}

	// NB: resetting the value map will already finalize all copies of the keys.
	v.valuesMap.Reset()

	if v.pool == nil {
		return
	}

	v.pool.Put(*v)
}

func (v *AggregateValues) addValue(value ident.ID) error {
	// NB(r): Allow for empty values to be set, an empty string
	// is still different from not having the field at all.
	bytesID := ident.BytesID(value.Bytes())

	// NB: fine to overwrite the values here.
	v.valuesMap.Set(bytesID, struct{}{})
	return nil
}
