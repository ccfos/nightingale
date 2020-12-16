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
	"bytes"
	"sort"
)

// Allow returns true if the given term satisfies the filter.
func (f AggregateFieldFilter) Allow(term []byte) bool {
	if len(f) == 0 {
		// NB: if filter is empty, all values are valid.
		return true
	}

	for _, allowed := range f {
		if bytes.Equal(term, allowed) {
			return true
		}
	}

	return false
}

// AddIfMissing adds the provided field if it's missing from the filter.
func (f AggregateFieldFilter) AddIfMissing(field []byte) AggregateFieldFilter {
	for _, fi := range f {
		if bytes.Equal(fi, field) {
			return f
		}
	}
	f = append(f, field)
	return f
}

// SortAndDedupe sorts and de-dupes the fields in the filter.
func (f AggregateFieldFilter) SortAndDedupe() AggregateFieldFilter {
	// short-circuit if possible.
	if len(f) <= 1 {
		return f
	}
	// sort the provided filter fields in order.
	sort.Slice(f, func(i, j int) bool {
		return bytes.Compare(f[i], f[j]) < 0
	})
	// ensure successive elements are not the same.
	deduped := make(AggregateFieldFilter, 0, len(f))
	if len(f) > 0 {
		deduped = append(deduped, f[0])
	}
	for i := 1; i < len(f); i++ {
		if bytes.Equal(f[i-1], f[i]) {
			continue
		}
		deduped = append(deduped, f[i])
	}
	return deduped
}
