// Copyright (c) 2020 Uber Technologies, Inc.
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

// Package unsafe contains operations that step around the type safety of Go programs.
package unsafe

import (
	_ "unsafe" // needed for go:linkname hack
)

//go:linkname fastrand runtime.fastrand
func fastrand() uint32

// Fastrandn returns a uint32 in range of 0..n, like rand() % n, but faster not as precise,
// https://lemire.me/blog/2016/06/27/a-fast-alternative-to-the-modulo-reduction/
func Fastrandn(n uint32) uint32 {
	// This is similar to fastrand() % n, but faster.
	return uint32(uint64(fastrand()) * uint64(n) >> 32)
}
