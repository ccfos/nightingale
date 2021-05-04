// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gosnmp

import (
	"bytes"
	"errors"
	"fmt"
)

func marshalObjectIdentifier(oid []int) (ret []byte, err error) {
	out := bytes.NewBuffer(make([]byte, 0, 128))
	if len(oid) < 2 || oid[0] > 6 || oid[1] >= 40 {
		return nil, errors.New("invalid object identifier")
	}

	err = out.WriteByte(byte(oid[0]*40 + oid[1]))
	if err != nil {
		return
	}
	for i := 2; i < len(oid); i++ {
		err = marshalBase128Int(out, int64(oid[i]))
		if err != nil {
			return
		}
	}

	ret = out.Bytes()

	return
}

// parseObjectIdentifier parses an OBJECT IDENTIFIER from the given bytes and
// returns it. An object identifier is a sequence of variable length integers
// that are assigned in a hierarchy.
func parseObjectIdentifier(bytes []byte) (s []int, err error) {
	if len(bytes) == 0 {
		err = fmt.Errorf("zero length OBJECT IDENTIFIER")
		return
	}

	// In the worst case, we get two elements from the first byte (which is
	// encoded differently) and then every varint is a single byte long.
	s = make([]int, len(bytes)+1)

	// The first byte is 40*value1 + value2:
	s[0] = int(bytes[0]) / 40
	s[1] = int(bytes[0]) % 40
	i := 2
	for offset := 1; offset < len(bytes); i++ {
		var v int
		v, offset, err = parseBase128Int(bytes, offset)
		if err != nil {
			return
		}
		s[i] = v
	}
	s = s[0:i]
	return
}

// parseBase128Int parses a base-128 encoded int from the given offset in the
// given byte slice. It returns the value and the new offset.
func parseBase128Int(bytes []byte, initOffset int) (ret, offset int, err error) {
	offset = initOffset
	for shifted := 0; offset < len(bytes); shifted++ {
		if shifted > 4 {
			err = fmt.Errorf("Structural Error: base 128 integer too large")
			return
		}
		ret <<= 7
		b := bytes[offset]
		ret |= int(b & 0x7f)
		offset++
		if b&0x80 == 0 {
			return
		}
	}
	err = fmt.Errorf("Syntax Error: truncated base 128 integer")
	return
}

func marshalBase128Int(out *bytes.Buffer, n int64) (err error) {
	if n == 0 {
		err = out.WriteByte(0)
		return
	}

	l := 0
	for i := n; i > 0; i >>= 7 {
		l++
	}

	for i := l - 1; i >= 0; i-- {
		o := byte(n >> uint(i*7))
		o &= 0x7f
		if i != 0 {
			o |= 0x80
		}
		err = out.WriteByte(o)
		if err != nil {
			return
		}
	}

	return nil
}

// parseInt64 treats the given bytes as a big-endian, signed integer and
// returns the result.
func parseInt64(bytes []byte) (ret uint64, err error) {
	if len(bytes) > 8 {
		// We'll overflow an int64 in this case.
		err = errors.New("integer too large")
		return
	}
	for bytesRead := 0; bytesRead < len(bytes); bytesRead++ {
		ret <<= 8
		ret |= uint64(bytes[bytesRead])
	}

	// Shift up and down in order to sign extend the result.
	ret <<= 64 - uint8(len(bytes))*8
	ret >>= 64 - uint8(len(bytes))*8
	return
}

// parseInt treats the given bytes as a big-endian, signed integer and returns
// the result.
func parseInt(bytes []byte) (int, error) {
	ret64, err := parseInt64(bytes)
	if err != nil {
		return 0, err
	}
	if ret64 != uint64(int(ret64)) {
		return 0, errors.New("integer too large")
	}
	return int(ret64), nil
}

func Uvarint(buf []byte) (x uint64) {
	for i, b := range buf {
		x = x<<8 + uint64(b)
		if i == 7 {
			return
		}
	}
	return
}

// BIT STRING

// BitStringValue is the structure to use when you want an ASN.1 BIT STRING type. A
// bit string is padded up to the nearest byte in memory and the number of
// valid bits is recorded. Padding bits will be zero.
type BitStringValue struct {
	Bytes     []byte // bits packed into bytes.
	BitLength int    // length in bits.
}

// At returns the bit at the given index. If the index is out of range it
// returns false.
func (b BitStringValue) At(i int) int {
	if i < 0 || i >= b.BitLength {
		return 0
	}
	x := i / 8
	y := 7 - uint(i%8)
	return int(b.Bytes[x]>>y) & 1
}

// RightAlign returns a slice where the padding bits are at the beginning. The
// slice may share memory with the BitString.
func (b BitStringValue) RightAlign() []byte {
	shift := uint(8 - (b.BitLength % 8))
	if shift == 8 || len(b.Bytes) == 0 {
		return b.Bytes
	}

	a := make([]byte, len(b.Bytes))
	a[0] = b.Bytes[0] >> shift
	for i := 1; i < len(b.Bytes); i++ {
		a[i] = b.Bytes[i-1] << (8 - shift)
		a[i] |= b.Bytes[i] >> shift
	}

	return a
}

// parseBitString parses an ASN.1 bit string from the given byte slice and returns it.
func parseBitString(bytes []byte) (ret BitStringValue, err error) {
	if len(bytes) == 0 {
		err = errors.New("zero length BIT STRING")
		return
	}
	paddingBits := int(bytes[0])
	if paddingBits > 7 ||
		len(bytes) == 1 && paddingBits > 0 ||
		bytes[len(bytes)-1]&((1<<bytes[0])-1) != 0 {
		err = errors.New("invalid padding bits in BIT STRING")
		return
	}
	ret.BitLength = (len(bytes)-1)*8 - paddingBits
	ret.Bytes = bytes[1:]
	return
}
