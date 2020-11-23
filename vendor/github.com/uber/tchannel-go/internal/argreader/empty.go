// Copyright (c) 2015 Uber Technologies, Inc.

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

package argreader

import (
	"fmt"
	"io"
	"sync"
)

var _bufPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 128)
		return &b
	},
}

// EnsureEmpty ensures that the specified reader is empty. If the reader is
// not empty, it returns an error with the specified stage in the message.
func EnsureEmpty(r io.Reader, stage string) error {
	buf := _bufPool.Get().(*[]byte)
	defer _bufPool.Put(buf)

	n, err := r.Read(*buf)
	if n > 0 {
		return fmt.Errorf("found unexpected bytes after %s, found (upto 128 bytes): %x", stage, (*buf)[:n])
	}
	if err == io.EOF {
		return nil
	}
	return err
}
