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

package graphite

// DropLastMetricPart returns the metric string without the last segment.
func DropLastMetricPart(metric string) string {
	// read string in reverse until encountering a delimiter
	for i := len(metric) - 1; i >= 0; i-- {
		if metric[i] == '.' {
			return metric[:i]
		}
	}

	return metric[:0]
}

// CountMetricParts counts the number of segments in the given metric string.
func CountMetricParts(metric string) int {
	return countMetricPartsWithDelimiter(metric, '.')
}

func countMetricPartsWithDelimiter(metric string, delim byte) int {
	if len(metric) == 0 {
		return 0
	}

	count := 1
	for i := 0; i < len(metric); i++ {
		if metric[i] == delim {
			count++
		}
	}

	return count
}

// ExtractNthMetricPart returns the nth part of the metric string. Index starts from 0
// and assumes metrics are delimited by '.'. If n is negative or bigger than the number
// of parts, returns an empty string.
func ExtractNthMetricPart(metric string, n int) string {
	return ExtractNthStringPart(metric, n, '.')
}

// ExtractNthStringPart returns the nth part of the metric string. Index starts from 0.
// If n is negative or bigger than the number of parts, returns an empty string.
func ExtractNthStringPart(target string, n int, delim rune) string {
	if n < 0 {
		return ""
	}

	leftSide := 0
	delimsToGo := n + 1
	for i := 0; i < len(target); i++ {
		if target[i] == byte(delim) {
			delimsToGo--
			if delimsToGo == 0 {
				return target[leftSide:i]
			}
			leftSide = i + 1
		}
	}

	if delimsToGo > 1 {
		return ""
	}

	return target[leftSide:]
}
