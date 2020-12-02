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

package tchannel

import (
	"hash"
	"hash/crc32"
	"sync"
)

var checksumPools [checksumCount]sync.Pool

// A ChecksumType is a checksum algorithm supported by TChannel for checksumming call bodies
type ChecksumType byte

const (
	// ChecksumTypeNone indicates no checksum is included in the message
	ChecksumTypeNone ChecksumType = 0

	// ChecksumTypeCrc32 indicates the message checksum is calculated using crc32
	ChecksumTypeCrc32 ChecksumType = 1

	// ChecksumTypeFarmhash indicates the message checksum is calculated using Farmhash
	ChecksumTypeFarmhash ChecksumType = 2

	// ChecksumTypeCrc32C indicates the message checksum is calculated using crc32c
	ChecksumTypeCrc32C ChecksumType = 3

	checksumCount = 4
)

func init() {
	crc32CastagnoliTable := crc32.MakeTable(crc32.Castagnoli)

	ChecksumTypeNone.pool().New = func() interface{} {
		return nullChecksum{}
	}
	ChecksumTypeCrc32.pool().New = func() interface{} {
		return newHashChecksum(ChecksumTypeCrc32, crc32.NewIEEE())
	}
	ChecksumTypeCrc32C.pool().New = func() interface{} {
		return newHashChecksum(ChecksumTypeCrc32C, crc32.New(crc32CastagnoliTable))
	}

	// TODO: Implement farm hash.
	ChecksumTypeFarmhash.pool().New = func() interface{} {
		return nullChecksum{}
	}
}

// ChecksumSize returns the size in bytes of the checksum calculation
func (t ChecksumType) ChecksumSize() int {
	switch t {
	case ChecksumTypeNone:
		return 0
	case ChecksumTypeCrc32, ChecksumTypeCrc32C:
		return crc32.Size
	case ChecksumTypeFarmhash:
		return 4
	default:
		return 0
	}
}

// pool returns the sync.Pool used to pool checksums for this type.
func (t ChecksumType) pool() *sync.Pool {
	return &checksumPools[int(t)]
}

// New creates a new Checksum of the given type
func (t ChecksumType) New() Checksum {
	s := t.pool().Get().(Checksum)
	s.Reset()
	return s
}

// Release puts a Checksum back in the pool.
func (t ChecksumType) Release(checksum Checksum) {
	t.pool().Put(checksum)
}

// A Checksum calculates a running checksum against a bytestream
type Checksum interface {
	// TypeCode returns the type of this checksum
	TypeCode() ChecksumType

	// Size returns the size of the calculated checksum
	Size() int

	// Add adds bytes to the checksum calculation
	Add(b []byte) []byte

	// Sum returns the current checksum value
	Sum() []byte

	// Release puts a Checksum back in the pool.
	Release()

	// Reset resets the checksum state to the default 0 value.
	Reset()
}

// No checksum
type nullChecksum struct{}

// TypeCode returns the type of the checksum
func (c nullChecksum) TypeCode() ChecksumType { return ChecksumTypeNone }

// Size returns the size of the checksum data, in the case the null checksum this is zero
func (c nullChecksum) Size() int { return 0 }

// Add adds a byteslice to the checksum calculation
func (c nullChecksum) Add(b []byte) []byte { return nil }

// Sum returns the current checksum calculation
func (c nullChecksum) Sum() []byte { return nil }

// Release puts a Checksum back in the pool.
func (c nullChecksum) Release() {
	c.TypeCode().Release(c)
}

// Reset resets the checksum state to the default 0 value.
func (c nullChecksum) Reset() {}

// Hash Checksum
type hashChecksum struct {
	checksumType ChecksumType
	hash         hash.Hash
	sumCache     []byte
}

func newHashChecksum(t ChecksumType, hash hash.Hash) *hashChecksum {
	return &hashChecksum{
		checksumType: t,
		hash:         hash,
		sumCache:     make([]byte, 0, 4),
	}
}

// TypeCode returns the type of the checksum
func (h *hashChecksum) TypeCode() ChecksumType { return h.checksumType }

// Size returns the size of the checksum data
func (h *hashChecksum) Size() int { return h.hash.Size() }

// Add adds a byte slice to the checksum calculation
func (h *hashChecksum) Add(b []byte) []byte { h.hash.Write(b); return h.Sum() }

// Sum returns the current value of the checksum calculation
func (h *hashChecksum) Sum() []byte { return h.hash.Sum(h.sumCache) }

// Release puts a Checksum back in the pool.
func (h *hashChecksum) Release() { h.TypeCode().Release(h) }

// Reset resets the checksum state to the default 0 value.
func (h *hashChecksum) Reset() { h.hash.Reset() }
