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

package encoding

import (
	"encoding/binary"
	"errors"
	"io"
)

const maxInt = int(^uint(0) >> 1)

var byteOrder = binary.LittleEndian

var errUvarintOverflow = errors.New("uvarint overflows 64 bits")
var errIntOverflow = errors.New("decoded integer overflows an int")

// Encoder is a low-level encoder that can be used for encoding basic types.
type Encoder struct {
	buf []byte
	tmp [binary.MaxVarintLen64]byte
}

// NewEncoder returns a new encoder.
func NewEncoder(n int) *Encoder {
	return &Encoder{buf: make([]byte, 0, n)}
}

// Bytes returns the encoded bytes.
func (e *Encoder) Bytes() []byte { return e.buf }

// Len returns the length of the encoder.
func (e *Encoder) Len() int { return len(e.buf) }

// Reset resets the encoder.
func (e *Encoder) Reset() { e.buf = e.buf[:0] }

// PutUint32 encodes a uint32 and returns the number of bytes written which is always 4.
func (e *Encoder) PutUint32(x uint32) int {
	byteOrder.PutUint32(e.tmp[:], x)
	e.buf = append(e.buf, e.tmp[:4]...)
	return 4
}

// PutUint64 encodes a uint64 and returns the number of bytes written which is always 8.
func (e *Encoder) PutUint64(x uint64) int {
	byteOrder.PutUint64(e.tmp[:], x)
	e.buf = append(e.buf, e.tmp[:8]...)
	return 8
}

// PutUvarint encodes a variable-sized unsigned integer and returns the number of
// bytes written.
func (e *Encoder) PutUvarint(x uint64) int {
	n := binary.PutUvarint(e.tmp[:], x)
	e.buf = append(e.buf, e.tmp[:n]...)
	return n
}

// PutBytes encodes a byte slice and returns the number of bytes written.
func (e *Encoder) PutBytes(b []byte) int {
	n := e.PutUvarint(uint64(len(b)))
	e.buf = append(e.buf, b...)
	return n + len(b)
}

// Decoder is a low-level decoder for decoding basic types.
type Decoder struct {
	buf []byte
}

// NewDecoder returns a new Decoder.
func NewDecoder(buf []byte) *Decoder {
	return &Decoder{buf: buf}
}

// Reset resets the decoder.
func (d *Decoder) Reset(buf []byte) { d.buf = buf }

// Uint32 reads a uint32 from the decoder.
func (d *Decoder) Uint32() (uint32, error) {
	if len(d.buf) < 4 {
		return 0, io.ErrShortBuffer
	}
	x := byteOrder.Uint32(d.buf)
	d.buf = d.buf[4:]
	return x, nil
}

// Uint64 reads a uint64 from the decoder.
func (d *Decoder) Uint64() (uint64, error) {
	if len(d.buf) < 8 {
		return 0, io.ErrShortBuffer
	}
	x := byteOrder.Uint64(d.buf)
	d.buf = d.buf[8:]
	return x, nil
}

// Uvarint reads a variable-sized unsigned integer.
func (d *Decoder) Uvarint() (uint64, error) {
	x, n := binary.Uvarint(d.buf)
	if n == 0 {
		return 0, io.ErrShortBuffer
	}
	if n < 0 {
		return 0, errUvarintOverflow
	}
	d.buf = d.buf[n:]
	return x, nil
}

// Bytes reads a byte slice from the decoder.
func (d *Decoder) Bytes() ([]byte, error) {
	x, err := d.Uvarint()
	if err != nil {
		return nil, err
	}

	// Verify the length of the slice won't overflow an int.
	if x > uint64(maxInt) {
		return nil, errIntOverflow
	}

	n := int(x)
	if len(d.buf) < n {
		return nil, io.ErrShortBuffer
	}
	b := d.buf[:n]
	d.buf = d.buf[n:]
	return b, nil
}
