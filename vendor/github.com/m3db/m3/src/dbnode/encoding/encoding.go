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

package encoding

import "math/bits"

// Bit is just a byte
type Bit byte

// NumSig returns the number of significant values in a uint64
func NumSig(v uint64) uint8 {
	if v == 0 {
		return 0
	}

	numLeading := uint8(0)
	for tmp := v; (tmp & (1 << 63)) == 0; tmp <<= 1 {
		numLeading++
	}

	return uint8(64) - numLeading
}

// LeadingAndTrailingZeros calculates the number of leading and trailing 0s
// for a uint64
func LeadingAndTrailingZeros(v uint64) (int, int) {
	if v == 0 {
		return 64, 0
	}

	numLeading := bits.LeadingZeros64(v)
	numTrailing := bits.TrailingZeros64(v)
	return numLeading, numTrailing
}

// SignExtend sign extends the highest bit of v which has numBits (<=64)
func SignExtend(v uint64, numBits uint) int64 {
	shift := 64 - numBits
	return (int64(v) << shift) >> shift
}
