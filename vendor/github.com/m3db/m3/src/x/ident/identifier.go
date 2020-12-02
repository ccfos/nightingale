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

package ident

import (
	"bytes"

	"github.com/m3db/m3/src/x/checked"
)

// BinaryID constructs a new ID based on a binary value.
func BinaryID(v checked.Bytes) ID {
	v.IncRef()
	return &id{data: v}
}

// StringID constructs a new ID based on a string value.
func StringID(str string) ID {
	return BytesID([]byte(str))
}

type id struct {
	data       checked.Bytes
	pool       Pool
	noFinalize bool
}

// Bytes directly returns the underlying bytes of an ID, it is not safe
// to hold a reference to this slice and is only valid during the lifetime
// of the the ID itself.
func (v *id) Bytes() []byte {
	if v.data == nil {
		return nil
	}
	return v.data.Bytes()
}

func (v *id) Equal(value ID) bool {
	return bytes.Equal(v.Bytes(), value.Bytes())
}

func (v *id) NoFinalize() {
	v.noFinalize = true
}

func (v *id) IsNoFinalize() bool {
	return v.noFinalize
}

func (v *id) Finalize() {
	if v.noFinalize {
		return
	}
	v.data.DecRef()
	v.data.Finalize()
	v.data = nil

	if v.pool == nil {
		return
	}

	v.pool.Put(v)
}

func (v *id) String() string {
	return string(v.Bytes())
}
