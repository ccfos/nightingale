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

package util

// Memset is a faster way to initialize a float64 array.
// NB: Inspired from https://github.com/tmthrgd/go-memset, which works
// directly on the byte interface. The 0 case is optimized due to
// https://github.com/golang/go/issues/5373 but for non the zero case,
// we use the copy() optimization.
// BenchmarkMemsetZeroValues-4    1000000   1344 ns/op
// BenchmarkLoopZeroValues-4       500000   3217 ns/op
// BenchmarkMemsetNonZeroValues-4 1000000   1537 ns/op
// BenchmarkLoopNonZeroValues-4    500000   3236 ns/op
func Memset(data []float64, value float64) {
	if value == 0 {
		for i := range data {
			data[i] = 0
		}
	} else if len(data) != 0 {
		data[0] = value

		for i := 1; i < len(data); i *= 2 {
			copy(data[i:], data[:i])
		}
	}
}

// MemsetInt is a faster way to initialize an int array.
func MemsetInt(data []int, value int) {
	if value == 0 {
		for i := range data {
			data[i] = 0
		}
	} else if len(data) != 0 {
		data[0] = value

		for i := 1; i < len(data); i *= 2 {
			copy(data[i:], data[:i])
		}
	}
}
