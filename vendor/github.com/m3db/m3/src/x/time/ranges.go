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

package time

import (
	"bytes"
	"container/list"
)

// Ranges is a collection of time ranges.
type Ranges interface {
	AddRange(Range)
	AddRanges(Ranges)
	RemoveRange(Range)
	RemoveRanges(Ranges)
	Overlaps(Range) bool
	Iter() *RangeIter
	Clone() Ranges
	Len() int
	IsEmpty() bool
	String() string
}

type ranges struct {
	sortedRanges *list.List
}

// NewRanges constructs a new Ranges object comprising the provided ranges.
func NewRanges(in ...Range) Ranges {
	res := &ranges{sortedRanges: list.New()}
	for _, r := range in {
		res.AddRange(r)
	}
	return res
}

// Len returns the number of ranges included.
func (tr *ranges) Len() int {
	return tr.sortedRanges.Len()
}

// IsEmpty returns true if the list of time ranges is empty.
func (tr *ranges) IsEmpty() bool {
	return tr.Len() == 0
}

// Overlaps checks if the range overlaps with any of the ranges in the collection.
func (tr *ranges) Overlaps(r Range) bool {
	if r.IsEmpty() {
		return false
	}
	e := tr.findFirstNotBefore(r)
	if e == nil {
		return false
	}
	lr := e.Value.(Range)
	return lr.Overlaps(r)
}

// AddRange adds the time range to the collection of ranges.
func (tr *ranges) AddRange(r Range) {
	tr.addRangeInPlace(r)
}

// AddRanges adds the time ranges.
func (tr *ranges) AddRanges(other Ranges) {
	it := other.Iter()
	for it.Next() {
		tr.addRangeInPlace(it.Value())
	}
}

// RemoveRange removes the time range from the collection of ranges.
func (tr *ranges) RemoveRange(r Range) {
	tr.removeRangeInPlace(r)
}

// RemoveRanges removes the given time ranges from the current one.
func (tr *ranges) RemoveRanges(other Ranges) {
	it := other.Iter()
	for it.Next() {
		tr.removeRangeInPlace(it.Value())
	}
}

// Iter returns an iterator that iterates over the time ranges included.
func (tr *ranges) Iter() *RangeIter {
	return newRangeIter(tr.sortedRanges)
}

// Clone makes a clone of the time ranges.
func (tr *ranges) Clone() Ranges {
	res := &ranges{sortedRanges: list.New()}
	for e := tr.sortedRanges.Front(); e != nil; e = e.Next() {
		res.sortedRanges.PushBack(e.Value.(Range))
	}
	return res
}

// String returns the string representation of the range.
func (tr *ranges) String() string {
	var buf bytes.Buffer
	buf.WriteString("[")
	for e := tr.sortedRanges.Front(); e != nil; e = e.Next() {
		buf.WriteString(e.Value.(Range).String())
		if e.Next() != nil {
			buf.WriteString(",")
		}
	}
	buf.WriteString("]")
	return buf.String()
}

// addRangeInPlace adds r to tr in place without creating a new copy.
func (tr *ranges) addRangeInPlace(r Range) {
	if r.IsEmpty() {
		return
	}

	e := tr.findFirstNotBefore(r)
	for e != nil {
		lr := e.Value.(Range)
		ne := e.Next()
		if !lr.Overlaps(r) {
			break
		}
		r = r.Merge(lr)
		tr.sortedRanges.Remove(e)
		e = ne
	}
	if e == nil {
		tr.sortedRanges.PushBack(r)
		return
	}
	tr.sortedRanges.InsertBefore(r, e)
}

func (tr *ranges) removeRangeInPlace(r Range) {
	if r.IsEmpty() {
		return
	}
	e := tr.findFirstNotBefore(r)
	for e != nil {
		lr := e.Value.(Range)
		ne := e.Next()
		if !lr.Overlaps(r) {
			return
		}
		res := lr.Subtract(r)
		if res == nil {
			tr.sortedRanges.Remove(e)
		} else {
			e.Value = res[0]
			if len(res) == 2 {
				tr.sortedRanges.InsertAfter(res[1], e)
			}
		}
		e = ne
	}
}

// findFirstNotBefore finds the first interval that's not before r.
func (tr *ranges) findFirstNotBefore(r Range) *list.Element {
	if tr.sortedRanges == nil {
		return nil
	}
	for e := tr.sortedRanges.Front(); e != nil; e = e.Next() {
		if !e.Value.(Range).Before(r) {
			return e
		}
	}
	return nil
}
