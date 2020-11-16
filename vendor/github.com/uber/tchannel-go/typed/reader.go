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

package typed

import (
	"encoding/binary"
	"io"
	"sync"
)

const maxPoolStringLen = 32

// Reader is a reader that reads typed values from an io.Reader.
type Reader struct {
	reader io.Reader
	err    error
	buf    [maxPoolStringLen]byte
}

var readerPool = sync.Pool{
	New: func() interface{} {
		return &Reader{}
	},
}

// NewReader returns a reader that reads typed values from the reader.
func NewReader(reader io.Reader) *Reader {
	r := readerPool.Get().(*Reader)
	r.reader = reader
	r.err = nil
	return r
}

// ReadUint16 reads a uint16.
func (r *Reader) ReadUint16() uint16 {
	if r.err != nil {
		return 0
	}

	buf := r.buf[:2]

	var readN int
	readN, r.err = io.ReadFull(r.reader, buf)
	if readN < 2 {
		return 0
	}
	return binary.BigEndian.Uint16(buf)
}

// ReadString reads a string of length n.
func (r *Reader) ReadString(n int) string {
	if r.err != nil {
		return ""
	}

	var buf []byte
	if n <= maxPoolStringLen {
		buf = r.buf[:n]
	} else {
		buf = make([]byte, n)
	}

	var readN int
	readN, r.err = io.ReadFull(r.reader, buf)
	if readN < n {
		return ""
	}
	s := string(buf)

	return s
}

// ReadLen16String reads a uint16-length prefixed string.
func (r *Reader) ReadLen16String() string {
	len := r.ReadUint16()
	return r.ReadString(int(len))
}

// Err returns any errors hit while reading from the underlying reader.
func (r *Reader) Err() error {
	return r.err
}

// Release puts the Reader back in the pool.
func (r *Reader) Release() {
	readerPool.Put(r)
}
