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
	"errors"

	"github.com/m3db/m3/src/m3ninx/index/segment"
)

var (
	errNoFiltersSpecified = errors.New("no fields specified to filter upon")
)

func newFilterFieldsIterator(
	reader segment.Reader,
	fields AggregateFieldFilter,
) (segment.FieldsIterator, error) {
	if len(fields) == 0 {
		return nil, errNoFiltersSpecified
	}
	return &filterFieldsIterator{
		reader:     reader,
		fields:     fields,
		currentIdx: -1,
	}, nil
}

type filterFieldsIterator struct {
	reader segment.Reader
	fields AggregateFieldFilter

	err        error
	currentIdx int
}

var _ segment.FieldsIterator = &filterFieldsIterator{}

func (f *filterFieldsIterator) Next() bool {
	if f.err != nil {
		return false
	}

	f.currentIdx++ // required because we start at -1
	for f.currentIdx < len(f.fields) {
		field := f.fields[f.currentIdx]

		ok, err := f.reader.ContainsField(field)
		if err != nil {
			f.err = err
			return false
		}

		// i.e. we found a field from the filter list contained in the segment.
		if ok {
			return true
		}

		// the current field is unsuitable, so we skip to the next possiblity.
		f.currentIdx++
	}

	return false
}

func (f *filterFieldsIterator) Current() []byte { return f.fields[f.currentIdx] }
func (f *filterFieldsIterator) Err() error      { return f.err }
func (f *filterFieldsIterator) Close() error    { return nil }
