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

package xpool

import (
	"github.com/m3db/m3/src/x/checked"
)

// CheckedBytesWrapperPool allows users to wrap []byte which have been
// allocated by other libraries (e.g. thrift/t-channel) with a checked.Bytes
// abstraction. This is to ensure we reference count usage, and more importantly,
// can interop with the rest of the M3DB codebase (which requires checked.Bytes).
// NB: This pool doesn't require a Put method, as Finalizing the bytes returned from
// Get does the Put internally.
type CheckedBytesWrapperPool interface {
	Init()

	Get([]byte) checked.Bytes
}
