// Copyright (c) 2019 Uber Technologies, Inc.
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

// This file is mostly copy-pasta from: https://github.com/jhump/protoreflect/blob/master/dynamic/codec.go
// since the jhump/protoreflect library does not expose the `codedBuffer` type.

package proto

import (
	"errors"
	"fmt"
	"io"
	"math"
)

var (
	// errOverflow is returned when an integer is too large to be represented.
	errOverflow = errors.New("proto: integer overflow")
)

// A reader/writer type that assists with decoding protobuf's binary representation.
// This code is largely a fork of proto.Buffer, which cannot be used because it has no exported
// field or method that provides access to its underlying reader index.
type buffer struct {
	buf   []byte
	index int
}

func newCodedBuffer(buf []byte) *buffer {
	return &buffer{buf: buf}
}

func (cb *buffer) reset(b []byte) {
	cb.buf = b
	cb.index = 0
}

func (cb *buffer) eof() bool {
	return cb.index >= len(cb.buf)
}

func (cb *buffer) skip(count int) (int, bool) {
	newIndex := cb.index + count
	if newIndex > len(cb.buf) {
		return 0, false
	}
	cb.index = newIndex
	return 0, true
}

func (cb *buffer) decodeVarintSlow() (x uint64, err error) {
	i := cb.index
	l := len(cb.buf)

	for shift := uint(0); shift < 64; shift += 7 {
		if i >= l {
			err = io.ErrUnexpectedEOF
			return
		}
		b := cb.buf[i]
		i++
		x |= (uint64(b) & 0x7F) << shift
		if b < 0x80 {
			cb.index = i
			return
		}
	}

	// The number is too large to represent in a 64-bit value.
	err = errOverflow
	return
}

// DecodeVarint reads a varint-encoded integer from the Buffer.
// This is the format for the
// int32, int64, uint32, uint64, bool, and enum
// protocol buffer types.
func (cb *buffer) decodeVarint() (uint64, error) {
	i := cb.index
	buf := cb.buf

	if i >= len(buf) {
		return 0, io.ErrUnexpectedEOF
	} else if buf[i] < 0x80 {
		cb.index++
		return uint64(buf[i]), nil
	} else if len(buf)-i < 10 {
		return cb.decodeVarintSlow()
	}

	var b uint64
	// we already checked the first byte
	x := uint64(buf[i]) - 0x80
	i++

	b = uint64(buf[i])
	i++
	x += b << 7
	if b&0x80 == 0 {
		goto done
	}
	x -= 0x80 << 7

	b = uint64(buf[i])
	i++
	x += b << 14
	if b&0x80 == 0 {
		goto done
	}
	x -= 0x80 << 14

	b = uint64(buf[i])
	i++
	x += b << 21
	if b&0x80 == 0 {
		goto done
	}
	x -= 0x80 << 21

	b = uint64(buf[i])
	i++
	x += b << 28
	if b&0x80 == 0 {
		goto done
	}
	x -= 0x80 << 28

	b = uint64(buf[i])
	i++
	x += b << 35
	if b&0x80 == 0 {
		goto done
	}
	x -= 0x80 << 35

	b = uint64(buf[i])
	i++
	x += b << 42
	if b&0x80 == 0 {
		goto done
	}
	x -= 0x80 << 42

	b = uint64(buf[i])
	i++
	x += b << 49
	if b&0x80 == 0 {
		goto done
	}
	x -= 0x80 << 49

	b = uint64(buf[i])
	i++
	x += b << 56
	if b&0x80 == 0 {
		goto done
	}
	x -= 0x80 << 56

	b = uint64(buf[i])
	i++
	x += b << 63
	if b&0x80 == 0 {
		goto done
	}
	// x -= 0x80 << 63 // Always zero.

	return 0, errOverflow

done:
	cb.index = i
	return x, nil
}

func (cb *buffer) decodeTagAndWireType() (tag int32, wireType int8, err error) {
	var v uint64
	v, err = cb.decodeVarint()
	if err != nil {
		return
	}
	// low 7 bits is wire type
	wireType = int8(v & 7)
	// rest is int32 tag number
	v = v >> 3
	if v > math.MaxInt32 {
		err = fmt.Errorf("tag number out of range: %d", v)
		return
	}
	tag = int32(v)
	return
}

// DecodeFixed64 reads a 64-bit integer from the Buffer.
// This is the format for the
// fixed64, sfixed64, and double protocol buffer types.
func (cb *buffer) decodeFixed64() (x uint64, err error) {
	// x, err already 0
	i := cb.index + 8
	if i < 0 || i > len(cb.buf) {
		err = io.ErrUnexpectedEOF
		return
	}
	cb.index = i

	x = uint64(cb.buf[i-8])
	x |= uint64(cb.buf[i-7]) << 8
	x |= uint64(cb.buf[i-6]) << 16
	x |= uint64(cb.buf[i-5]) << 24
	x |= uint64(cb.buf[i-4]) << 32
	x |= uint64(cb.buf[i-3]) << 40
	x |= uint64(cb.buf[i-2]) << 48
	x |= uint64(cb.buf[i-1]) << 56
	return
}

// DecodeFixed32 reads a 32-bit integer from the Buffer.
// This is the format for the
// fixed32, sfixed32, and float protocol buffer types.
func (cb *buffer) decodeFixed32() (x uint64, err error) {
	// x, err already 0
	i := cb.index + 4
	if i < 0 || i > len(cb.buf) {
		err = io.ErrUnexpectedEOF
		return
	}
	cb.index = i

	x = uint64(cb.buf[i-4])
	x |= uint64(cb.buf[i-3]) << 8
	x |= uint64(cb.buf[i-2]) << 16
	x |= uint64(cb.buf[i-1]) << 24
	return
}

func decodeZigZag32(v uint64) int32 {
	return int32((uint32(v) >> 1) ^ uint32((int32(v&1)<<31)>>31))
}

func decodeZigZag64(v uint64) int64 {
	return int64((v >> 1) ^ uint64((int64(v&1)<<63)>>63))
}

// These are not ValueDecoders: they produce an array of bytes or a string.
// bytes, embedded messages

// DecodeRawBytes reads a count-delimited byte buffer from the Buffer.
// This is the format used for the bytes protocol buffer
// type and for embedded messages.
func (cb *buffer) decodeRawBytes(alloc bool) (buf []byte, err error) {
	n, err := cb.decodeVarint()
	if err != nil {
		return nil, err
	}

	nb := int(n)
	if nb < 0 {
		return nil, fmt.Errorf("proto: bad byte length %d", nb)
	}
	end := cb.index + nb
	if end < cb.index || end > len(cb.buf) {
		return nil, io.ErrUnexpectedEOF
	}

	if !alloc {
		buf = cb.buf[cb.index:end]
		cb.index += nb
		return
	}

	buf = make([]byte, nb)
	copy(buf, cb.buf[cb.index:])
	cb.index += nb
	return
}
