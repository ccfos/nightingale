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

// Package time implement facilities for working with time.
package time

import "time"

const (
	nanosPerMillis = int64(time.Millisecond)
)

// ToNormalizedTime returns the normalized units of time given a time unit.
func ToNormalizedTime(t time.Time, u time.Duration) int64 {
	return t.UnixNano() / u.Nanoseconds()
}

// FromNormalizedTime returns the time given the normalized time units and the time unit.
func FromNormalizedTime(nt int64, u time.Duration) time.Time {
	return time.Unix(0, int64(u/time.Nanosecond)*nt)
}

// ToNormalizedDuration returns the normalized units of duration given a time unit.
func ToNormalizedDuration(d time.Duration, u time.Duration) int64 {
	return int64(d / u)
}

// FromNormalizedDuration returns the duration given the normalized time duration and a time unit.
func FromNormalizedDuration(nd int64, u time.Duration) time.Duration {
	return time.Duration(nd) * u
}

// ToNanoseconds converts a time to nanoseconds.
func ToNanoseconds(t time.Time) int64 {
	return t.UnixNano()
}

// FromNanoseconds converts nanoseconds to a time.
func FromNanoseconds(nsecs int64) time.Time {
	return time.Unix(0, nsecs)
}

// ToUnixMillis converts a time to milliseconds since Unix epoch
func ToUnixMillis(t time.Time) int64 {
	return t.UnixNano() / nanosPerMillis
}

// FromUnixMillis converts milliseconds since Unix epoch to a time
func FromUnixMillis(ms int64) time.Time {
	return time.Unix(0, ms*nanosPerMillis)
}

// Ceil returns the result of rounding t up to a multiple of d since
// the zero time.
func Ceil(t time.Time, d time.Duration) time.Time {
	res := t.Truncate(d)
	if res.Before(t) {
		res = res.Add(d)
	}
	return res
}

// MinTime returns the earlier one of t1 and t2.
func MinTime(t1, t2 time.Time) time.Time {
	if t1.Before(t2) {
		return t1
	}
	return t2
}

// MaxTime returns the later one of t1 and t2.
func MaxTime(t1, t2 time.Time) time.Time {
	if t1.After(t2) {
		return t1
	}
	return t2
}
