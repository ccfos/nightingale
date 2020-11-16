// Copyright (c) 2018 Uber Technologies, Inc.
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

package postings

import "errors"

var (
	errRangerIterClosed = errors.New("iterator has already been closed")
)

type rangeIter struct {
	startInclusive ID
	endExclusive   ID
	closed         bool
	started        bool
}

// NewRangeIterator returns a new Iterator over the specified range.
func NewRangeIterator(startInclusive, endExclusive ID) Iterator {
	return &rangeIter{
		startInclusive: startInclusive,
		endExclusive:   endExclusive,
	}
}

func (r *rangeIter) Next() bool {
	if r.closed {
		return false
	}
	if r.started {
		r.startInclusive++
	}
	r.started = true
	return r.startInclusive < r.endExclusive
}

func (r *rangeIter) Current() ID {
	return r.startInclusive
}

func (r *rangeIter) Err() error {
	return nil
}

func (r *rangeIter) Close() error {
	if r.closed {
		return errRangerIterClosed
	}
	r.closed = true
	return nil
}
