// Copyright (c) 2017 Uber Technologies, Inc.
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

import "time"

// UnixNano is used to indicate that an int64 stores a unix timestamp at
// nanosecond resolution
type UnixNano int64

// ToTime returns a time.ToTime from a UnixNano
func (u UnixNano) ToTime() time.Time {
	return time.Unix(0, int64(u))
}

// ToUnixNano returns a UnixNano from a time.Time
func ToUnixNano(t time.Time) UnixNano {
	return UnixNano(t.UnixNano())
}

// Before reports whether the time instant u is before t.
func (u UnixNano) Before(t UnixNano) bool {
	return u < t
}

// After reports whether the time instant u is after t.
func (u UnixNano) After(t UnixNano) bool {
	return u > t
}

// Equal reports whether the time instant u is equal to t.
func (u UnixNano) Equal(t UnixNano) bool {
	return u == t
}
