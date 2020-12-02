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

package id

import "fmt"

// ID is a metric id.
type ID interface {
	// Bytes returns the raw bytes for this id.
	Bytes() []byte

	// TagValue looks up the tag value for a tag name.
	TagValue(tagName []byte) ([]byte, bool)
}

// NameAndTagsFn returns the name and the tag pairs given an id.
type NameAndTagsFn func(id []byte) (name []byte, tags []byte, err error)

// NewIDFn creates a new metric ID based on the metric name and metric tag pairs.
type NewIDFn func(name []byte, tags []TagPair) []byte

// MatchIDFn determines whether an id is considered "matched" based on certain criteria.
type MatchIDFn func(name []byte, tags []byte) bool

// RawID is the raw metric id.
type RawID []byte

// String is the string representation of a raw id.
func (rid RawID) String() string { return string(rid) }

// ChunkedID is a three-part id.
type ChunkedID struct {
	Prefix []byte
	Data   []byte
	Suffix []byte
}

// String is the string representation of the chunked id.
func (cid ChunkedID) String() string {
	return fmt.Sprintf("%s%s%s", cid.Prefix, cid.Data, cid.Suffix)
}
