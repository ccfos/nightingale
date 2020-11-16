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

package encoding

import (
	"bufio"
	"io"
	"math"
)

// istream encapsulates a readable stream.
type istream struct {
	r         *bufio.Reader // encoded stream
	err       error         // error encountered
	current   byte          // current byte we are working off of
	buffer    []byte        // buffer for reading in multiple bytes
	remaining uint          // bits remaining in current to be read
}

// NewIStream creates a new Istream
func NewIStream(reader io.Reader, bufioSize int) IStream {
	return &istream{
		r: bufio.NewReaderSize(reader, bufioSize),
		// Buffer meant to hold uint64 size of bytes.
		buffer: make([]byte, 8),
	}
}

func (is *istream) ReadBit() (Bit, error) {
	if is.err != nil {
		return 0, is.err
	}
	if is.remaining == 0 {
		if err := is.readByteFromStream(); err != nil {
			return 0, err
		}
	}
	return Bit(is.consumeBuffer(1)), nil
}

func (is *istream) Read(b []byte) (int, error) {
	if is.remaining == 0 {
		// Optimized path for when the iterator is already aligned on a byte boundary. Avoids
		// all the bit manipulation and ReadByte() function calls.
		// Use ReadFull because the bufferedReader may not return the requested number of bytes.
		return io.ReadFull(is.r, b)
	}

	var (
		i   int
		err error
	)

	for ; i < len(b); i++ {
		b[i], err = is.ReadByte()
		if err != nil {
			return i, err
		}
	}
	return i, nil
}

func (is *istream) ReadByte() (byte, error) {
	if is.err != nil {
		return 0, is.err
	}
	remaining := is.remaining
	res := is.consumeBuffer(remaining)
	if remaining == 8 {
		return res, nil
	}
	if err := is.readByteFromStream(); err != nil {
		return 0, err
	}
	res = (res << uint(8-remaining)) | is.consumeBuffer(8-remaining)
	return res, nil
}

func (is *istream) ReadBits(numBits uint) (uint64, error) {
	if is.err != nil {
		return 0, is.err
	}
	var res uint64
	numBytes := numBits / 8
	if numBytes > 0 {
		// Use Read call rather than individual ReadByte calls since it has
		// optimized path for when the iterator is aligned on a byte boundary.
		bytes := is.buffer[0:numBytes]
		_, err := is.Read(bytes)
		if err != nil {
			return 0, err
		}
		for _, b := range bytes {
			res = (res << 8) | uint64(b)
		}
	}

	numBits = numBits % 8
	for numBits > 0 {
		// This is equivalent to calling is.ReadBit() in a loop but some manual inlining
		// has been performed to optimize this loop as its heavily used in the hot path.
		if is.remaining == 0 {
			if err := is.readByteFromStream(); err != nil {
				return 0, err
			}
		}

		numToRead := numBits
		if is.remaining < numToRead {
			numToRead = is.remaining
		}
		bits := is.current >> (8 - numToRead)
		is.current <<= numToRead
		is.remaining -= numToRead
		res = (res << uint64(numToRead)) | uint64(bits)
		numBits -= numToRead
	}
	return res, nil
}

func (is *istream) PeekBits(numBits uint) (uint64, error) {
	// check the last byte first
	if numBits <= is.remaining {
		return uint64(readBitsInByte(is.current, numBits)), nil
	}
	// now check the bytes buffered and read more if necessary.
	numBitsRead := is.remaining
	res := uint64(readBitsInByte(is.current, is.remaining))
	numBytesToRead := int(math.Ceil(float64(numBits-numBitsRead) / 8))
	bytesRead, err := is.r.Peek(numBytesToRead)
	if err != nil {
		return 0, err
	}
	for i := 0; i < numBytesToRead-1; i++ {
		res = (res << 8) | uint64(bytesRead[i])
		numBitsRead += 8
	}
	remainder := readBitsInByte(bytesRead[numBytesToRead-1], numBits-numBitsRead)
	res = (res << (numBits - numBitsRead)) | uint64(remainder)
	return res, nil
}

func (is *istream) RemainingBitsInCurrentByte() uint {
	return is.remaining
}

// readBitsInByte reads numBits in byte b.
func readBitsInByte(b byte, numBits uint) byte {
	return b >> (8 - numBits)
}

// consumeBuffer consumes numBits in is.current.
func (is *istream) consumeBuffer(numBits uint) byte {
	res := readBitsInByte(is.current, numBits)
	is.current <<= numBits
	is.remaining -= numBits
	return res
}

func (is *istream) readByteFromStream() error {
	is.current, is.err = is.r.ReadByte()
	is.remaining = 8
	return is.err
}

func (is *istream) Reset(r io.Reader) {
	is.r.Reset(r)
	is.err = nil
	is.current = 0
	is.remaining = 0
}
