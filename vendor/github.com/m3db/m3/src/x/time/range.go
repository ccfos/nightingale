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
	"fmt"
	"time"
)

// Range represents [start, end)
type Range struct {
	Start time.Time
	End   time.Time
}

// IsEmpty returns whether the time range is empty.
func (r Range) IsEmpty() bool {
	return r.Start.Equal(r.End)
}

// Equal returns whether two time ranges are equal.
func (r Range) Equal(other Range) bool {
	return r.Start.Equal(other.Start) && r.End.Equal(other.End)
}

// Before determines whether r is before other.
func (r Range) Before(other Range) bool {
	return !r.End.After(other.Start)
}

// After determines whether r is after other.
func (r Range) After(other Range) bool {
	return other.Before(r)
}

// Contains determines whether r contains other.
func (r Range) Contains(other Range) bool {
	return !r.Start.After(other.Start) && !r.End.Before(other.End)
}

// Overlaps determines whether r overlaps with other.
func (r Range) Overlaps(other Range) bool {
	return r.End.After(other.Start) && r.Start.Before(other.End)
}

// Duration returns the duration of the range.
func (r Range) Duration() time.Duration {
	return r.End.Sub(r.Start)
}

// Intersect calculates the intersection of the receiver range against the
// provided argument range iff there is an overlap between the two. It also
// returns a bool indicating if there was a valid intersection.
func (r Range) Intersect(other Range) (Range, bool) {
	if !r.Overlaps(other) {
		return Range{}, false
	}
	newRange := r
	if newRange.Start.Before(other.Start) {
		newRange.Start = other.Start
	}
	if newRange.End.After(other.End) {
		newRange.End = other.End
	}
	return newRange, true
}

// Since returns the time range since a given point in time.
func (r Range) Since(t time.Time) Range {
	if t.Before(r.Start) {
		return r
	}
	if t.After(r.End) {
		return Range{}
	}
	return Range{Start: t, End: r.End}
}

// Merge merges the two ranges if they overlap. Otherwise,
// the gap between them is included.
func (r Range) Merge(other Range) Range {
	start := MinTime(r.Start, other.Start)
	end := MaxTime(r.End, other.End)
	return Range{Start: start, End: end}
}

// Subtract removes the intersection between r and other
// from r, possibly splitting r into two smaller ranges.
func (r Range) Subtract(other Range) []Range {
	if !r.Overlaps(other) {
		return []Range{r}
	}
	if other.Contains(r) {
		return nil
	}
	var res []Range
	left := Range{r.Start, other.Start}
	right := Range{other.End, r.End}
	if r.Contains(other) {
		if !left.IsEmpty() {
			res = append(res, left)
		}
		if !right.IsEmpty() {
			res = append(res, right)
		}
		return res
	}
	if !r.Start.After(other.Start) {
		if !left.IsEmpty() {
			res = append(res, left)
		}
		return res
	}
	if !right.IsEmpty() {
		res = append(res, right)
	}
	return res
}

// IterateForward iterates through a time range by step size in the
// forwards direction.
func (r Range) IterateForward(stepSize time.Duration, f func(t time.Time) (shouldContinue bool)) {
	for t := r.Start; t.Before(r.End); t = t.Add(stepSize) {
		if shouldContinue := f(t); !shouldContinue {
			break
		}
	}
}

// IterateBackward iterates through a time range by step size in the
// backwards direction.
func (r Range) IterateBackward(stepSize time.Duration, f func(t time.Time) (shouldContinue bool)) {
	for t := r.End; t.After(r.Start); t = t.Add(-stepSize) {
		if shouldContinue := f(t); !shouldContinue {
			break
		}
	}
}

// String returns the string representation of the range.
func (r Range) String() string {
	return fmt.Sprintf("(%v,%v)", r.Start, r.End)
}
