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

package ident

import (
	"bytes"
)

// BytesID is a small utility type to avoid the heavy weight of a true ID
// implementation when using in high throughput places like keys in a map.
type BytesID []byte

// var declaration to ensure package type BytesID implements ID
var _ ID = BytesID(nil)

// Bytes returns the underlying byte slice of the bytes ID.
func (v BytesID) Bytes() []byte {
	return v
}

// String returns the bytes ID as a string.
func (v BytesID) String() string {
	return string(v)
}

// Equal returns whether the bytes ID is equal to a given ID.
func (v BytesID) Equal(value ID) bool {
	return bytes.Equal(value.Bytes(), v)
}

// NoFinalize is a no-op for a bytes ID as Finalize is already a no-op.
func (v BytesID) NoFinalize() {
}

// IsNoFinalize is always true since BytesID is not pooled.
func (v BytesID) IsNoFinalize() bool {
	return true
}

// Finalize is a no-op for a bytes ID as it has no associated pool.
func (v BytesID) Finalize() {
}

var _ ID = (*ReuseableBytesID)(nil)

// ReuseableBytesID is a reuseable bytes ID, use with extreme care in
// places where the lifecycle is known (there is no checking with this
// ID).
type ReuseableBytesID struct {
	bytes []byte
}

// NewReuseableBytesID returns a new reuseable bytes ID, use with extreme
// care in places where the lifecycle is known (there is no checking with
// this ID).
func NewReuseableBytesID() *ReuseableBytesID {
	return &ReuseableBytesID{}
}

// Reset resets the bytes ID for reuse, make sure there are zero references
// to this ID from any other data structure at this point.
func (i *ReuseableBytesID) Reset(bytes []byte) {
	i.bytes = bytes
}

// Bytes implements ID.
func (i *ReuseableBytesID) Bytes() []byte {
	return i.bytes
}

// Equal implements ID.
func (i *ReuseableBytesID) Equal(value ID) bool {
	return bytes.Equal(i.bytes, value.Bytes())
}

// NoFinalize implements ID.
func (i *ReuseableBytesID) NoFinalize() {
}

// IsNoFinalize implements ID.
func (i *ReuseableBytesID) IsNoFinalize() bool {
	// Reuseable bytes ID are always not able to not be finalized
	// as this ID is reused with reset.
	return false
}

// Finalize implements ID.
func (i *ReuseableBytesID) Finalize() {
	// Noop as it will be re-used.
}

// String returns the bytes ID as a string.
func (i *ReuseableBytesID) String() string {
	return string(i.bytes)
}
