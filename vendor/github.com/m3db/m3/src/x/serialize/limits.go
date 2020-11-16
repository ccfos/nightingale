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

package serialize

import "math"

var (
	// defaultMaxNumberTags is the maximum number of tags that can be encoded.
	defaultMaxNumberTags uint16 = math.MaxUint16

	// defaultMaxTagLiteralLength is the maximum length of a tag Name/Value.
	defaultMaxTagLiteralLength uint16 = math.MaxUint16
)

type limits struct {
	maxNumberTags       uint16
	maxTagLiteralLength uint16
}

// NewTagSerializationLimits returns a new TagSerializationLimits object.
func NewTagSerializationLimits() TagSerializationLimits {
	return &limits{
		maxNumberTags:       defaultMaxNumberTags,
		maxTagLiteralLength: defaultMaxTagLiteralLength,
	}
}

func (l *limits) SetMaxNumberTags(v uint16) TagSerializationLimits {
	lim := *l
	lim.maxNumberTags = v
	return &lim
}

func (l *limits) MaxNumberTags() uint16 {
	return l.maxNumberTags
}

func (l *limits) SetMaxTagLiteralLength(v uint16) TagSerializationLimits {
	lim := *l
	lim.maxTagLiteralLength = v
	return &lim
}

func (l *limits) MaxTagLiteralLength() uint16 {
	return l.maxTagLiteralLength
}
