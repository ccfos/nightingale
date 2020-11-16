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

func (cb *buffer) encodeTagAndWireType(tag int32, wireType int8) {
	v := uint64((int64(tag) << 3) | int64(wireType))
	cb.encodeVarint(v)
}

// EncodeVarint writes a varint-encoded integer to the Buffer.
// This is the format for the
// int32, int64, uint32, uint64, bool, and enum
// protocol buffer types.
func (cb *buffer) encodeVarint(x uint64) {
	for x >= 1<<7 {
		cb.buf = append(cb.buf, uint8(x&0x7f|0x80))
		x >>= 7
	}
	cb.buf = append(cb.buf, uint8(x))
}

// EncodeFixed64 writes a 64-bit integer to the Buffer.
// This is the format for the
// fixed64, sfixed64, and double protocol buffer types.
func (cb *buffer) encodeFixed64(x uint64) {
	cb.buf = append(cb.buf,
		uint8(x),
		uint8(x>>8),
		uint8(x>>16),
		uint8(x>>24),
		uint8(x>>32),
		uint8(x>>40),
		uint8(x>>48),
		uint8(x>>56))
}

// EncodeFixed32 writes a 32-bit integer to the Buffer.
// This is the format for the
// fixed32, sfixed32, and float protocol buffer types.
func (cb *buffer) encodeFixed32(x uint32) {
	cb.buf = append(cb.buf,
		uint8(x),
		uint8(x>>8),
		uint8(x>>16),
		uint8(x>>24))
}

// EncodeRawBytes writes a count-delimited byte buffer to the Buffer.
// This is the format used for the bytes protocol buffer
// type and for embedded messages.
func (cb *buffer) encodeRawBytes(b []byte) {
	cb.encodeVarint(uint64(len(b)))
	cb.buf = append(cb.buf, b...)
}

func (cb *buffer) append(b []byte) {
	cb.buf = append(cb.buf, b...)
}

func encodeZigZag32(v int32) uint64 {
	return uint64((uint32(v) << 1) ^ uint32((v >> 31)))
}

func encodeZigZag64(v int64) uint64 {
	return (uint64(v) << 1) ^ uint64(v>>63)
}
