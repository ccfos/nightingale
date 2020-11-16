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

package digest

import (
	"encoding/binary"
	"os"
)

const (
	// DigestLenBytes is the length of generated digests in bytes.
	DigestLenBytes = 4
)

var (
	// Endianness is little endian
	endianness = binary.LittleEndian
)

// Buffer is a byte slice that facilitates digest reading and writing.
type Buffer []byte

// NewBuffer creates a new digest buffer.
func NewBuffer() Buffer {
	return make([]byte, DigestLenBytes)
}

// WriteDigest writes a digest to the buffer.
func (b Buffer) WriteDigest(digest uint32) {
	endianness.PutUint32(b, digest)
}

// WriteDigestToFile writes a digest to the file.
func (b Buffer) WriteDigestToFile(fd *os.File, digest uint32) error {
	b.WriteDigest(digest)
	_, err := fd.Write(b)
	return err
}

// ReadDigest reads the digest from the buffer.
func (b Buffer) ReadDigest() uint32 {
	return endianness.Uint32(b)
}

// ReadDigestFromFile reads the digest from the file.
func (b Buffer) ReadDigestFromFile(fd *os.File) (uint32, error) {
	_, err := fd.Read(b)
	if err != nil {
		return 0, err
	}
	return b.ReadDigest(), nil
}

// ToBuffer converts a byte slice to a digest buffer.
func ToBuffer(buf []byte) Buffer {
	return Buffer(buf[:DigestLenBytes])
}
