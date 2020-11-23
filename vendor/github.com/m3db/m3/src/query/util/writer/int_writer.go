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

package writer

// IntLength determines the number of digits in a base 10 integer.
func IntLength(i int) int {
	if i == 0 {
		return 1
	}

	count := 0
	for ; i > 0; i /= 10 {
		count++
	}

	return count
}

// WriteInteger writes a base 10 integer to a buffer at a given index.
//
// NB: based on fmt.Printf handling of integers, specifically base 10 case.
func WriteInteger(dst []byte, value, idx int) int {
	// Because printing is easier right-to-left: format u into buf, ending at buf[i].
	// We could make things marginally faster by splitting the 32-bit case out
	// into a separate block but it's not worth the duplication, so u has 64 bits.
	// Use constants for the division and modulo for more efficient code.
	// Switch cases ordered by popularity.
	idx = idx + IntLength(value)
	finalIndex := idx
	for value >= 10 {
		idx--
		dst[idx] = byte(48 + value%10)
		next := value / 10
		value = next
	}

	dst[idx-1] = byte(48 + value)
	return finalIndex
}

// IntsLength determines the number of digits in a list of base 10 integers,
// accounting for separators between each integer.
func IntsLength(is []int) int {
	// initialize length accounting for separators.
	l := len(is) - 1
	for _, i := range is {
		l += IntLength(i)
	}

	return l
}

// WriteIntegers writes a slice of base 10 integer to a buffer at a given index,
// separating each value with the given separator, returning the index at which
// the write ends.
//
// NB: Ensure that there is sufficient space in the buffer to hold values and
// separators.
func WriteIntegers(dst []byte, values []int, sep byte, idx int) int {
	l := len(values) - 1
	for _, v := range values[:l] {
		idx = WriteInteger(dst, v, idx)
		dst[idx] = sep
		idx++
	}

	idx = WriteInteger(dst, values[l], idx)
	// Write the last integer.
	return idx
}
