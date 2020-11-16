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
	"github.com/m3db/m3/src/x/checked"
	"github.com/m3db/m3/src/x/pool"
)

const (
	initAllocSize = 1024
)

// ostream encapsulates a writable stream.
type ostream struct {
	// We want to use a checked.Bytes when transferring ownership of the buffer
	// of the ostream. Unfortunately, the accounting overhead of going through
	// the checked.Bytes for every write is massive. As a result, we store both
	// the rawBuffer that backs the checked.Bytes AND the checked.Bytes themselves
	// in this struct.
	//
	// That way, whenever we're writing to the buffer we can avoid the cost accounting
	// overhead entirely, but when the data needs to be transffered to another owner
	// we use the checked.Bytes, which is when the accounting really matters anyways.
	//
	// The rawBuffer and checked.Bytes may get out of sync as the rawBuffer is written to,
	// but thats fine because we perform a "repair" by resetting the checked.Bytes underlying
	// byte slice to be the rawBuffer whenever we expose a checked.Bytes to an external caller.
	rawBuffer []byte
	checked   checked.Bytes

	pos       int // how many bits have been used in the last byte
	bytesPool pool.CheckedBytesPool
}

// NewOStream creates a new Ostream
func NewOStream(
	bytes checked.Bytes,
	initAllocIfEmpty bool,
	bytesPool pool.CheckedBytesPool,
) OStream {
	if bytes == nil && initAllocIfEmpty {
		bytes = checked.NewBytes(make([]byte, 0, initAllocSize), nil)
	}

	stream := &ostream{bytesPool: bytesPool}
	stream.Reset(bytes)
	return stream
}

func (os *ostream) Len() int {
	return len(os.rawBuffer)
}

func (os *ostream) Empty() bool {
	return os.Len() == 0 && os.pos == 0
}

func (os *ostream) lastIndex() int {
	return os.Len() - 1
}

func (os *ostream) hasUnusedBits() bool {
	return os.pos > 0 && os.pos < 8
}

// grow appends the last byte of v to rawBuffer and sets pos to np.
func (os *ostream) grow(v byte, np int) {
	os.ensureCapacityFor(1)
	os.rawBuffer = append(os.rawBuffer, v)

	os.pos = np
}

// ensureCapacity ensures that there is at least capacity for n more bytes.
func (os *ostream) ensureCapacityFor(n int) {
	var (
		currCap      = cap(os.rawBuffer)
		currLen      = len(os.rawBuffer)
		availableCap = currCap - currLen
		missingCap   = n - availableCap
	)
	if missingCap <= 0 {
		// Already have enough capacity.
		return
	}

	newCap := max(cap(os.rawBuffer)*2, currCap+missingCap)
	if p := os.bytesPool; p != nil {
		newChecked := p.Get(newCap)
		newChecked.IncRef()
		newChecked.AppendAll(os.rawBuffer)

		if os.checked != nil {
			os.checked.DecRef()
			os.checked.Finalize()
		}

		os.checked = newChecked
		os.rawBuffer = os.checked.Bytes()
	} else {
		newRawBuffer := make([]byte, 0, newCap)
		newRawBuffer = append(newRawBuffer, os.rawBuffer...)
		os.rawBuffer = newRawBuffer

		os.checked = checked.NewBytes(os.rawBuffer, nil)
		os.checked.IncRef()
	}
}

func (os *ostream) fillUnused(v byte) {
	os.rawBuffer[os.lastIndex()] |= v >> uint(os.pos)
}

func (os *ostream) WriteBit(v Bit) {
	v <<= 7
	if !os.hasUnusedBits() {
		os.grow(byte(v), 1)
		return
	}
	os.fillUnused(byte(v))
	os.pos++
}

func (os *ostream) WriteByte(v byte) {
	if !os.hasUnusedBits() {
		os.grow(v, 8)
		return
	}
	os.fillUnused(v)
	os.grow(v<<uint(8-os.pos), os.pos)
}

func (os *ostream) WriteBytes(bytes []byte) {
	// Call ensureCapacityFor ahead of time to ensure that the bytes pool is used to
	// grow the rawBuffer (as opposed to append possibly triggering an allocation if
	// it wasn't) and that its only grown a maximum of one time regardless of the size
	// of the []byte being written.
	os.ensureCapacityFor(len(bytes))

	if !os.hasUnusedBits() {
		// If the stream is aligned on a byte boundary then all of the WriteByte()
		// function calls and bit-twiddling can be skipped in favor of a single
		// copy operation.
		os.rawBuffer = append(os.rawBuffer, bytes...)
		// Position 8 indicates that the last byte of the buffer has been completely
		// filled.
		os.pos = 8
		return
	}

	for i := 0; i < len(bytes); i++ {
		os.WriteByte(bytes[i])
	}
}

func (os *ostream) Write(bytes []byte) (int, error) {
	os.WriteBytes(bytes)
	return len(bytes), nil
}

func (os *ostream) WriteBits(v uint64, numBits int) {
	if numBits == 0 {
		return
	}

	// we should never write more than 64 bits for a uint64
	if numBits > 64 {
		numBits = 64
	}

	v <<= uint(64 - numBits)
	for numBits >= 8 {
		os.WriteByte(byte(v >> 56))
		v <<= 8
		numBits -= 8
	}

	for numBits > 0 {
		os.WriteBit(Bit((v >> 63) & 1))
		v <<= 1
		numBits--
	}
}

func (os *ostream) Discard() checked.Bytes {
	os.repairCheckedBytes()

	buffer := os.checked
	buffer.DecRef()

	os.rawBuffer = nil
	os.pos = 0
	os.checked = nil

	return buffer
}

func (os *ostream) Reset(buffer checked.Bytes) {
	if os.checked != nil {
		// Release ref of the current raw buffer
		os.checked.DecRef()
		os.checked.Finalize()

		os.rawBuffer = nil
		os.checked = nil
	}

	if buffer != nil {
		// Track ref to the new raw buffer
		buffer.IncRef()

		os.checked = buffer
		os.rawBuffer = os.checked.Bytes()
	}

	os.pos = 0
	if os.Len() > 0 {
		// If the byte array passed in is not empty, we set
		// pos to 8 indicating the last byte is fully used.
		os.pos = 8
	}
}

func (os *ostream) RawBytes() ([]byte, int) {
	return os.rawBuffer, os.pos
}

func (os *ostream) CheckedBytes() (checked.Bytes, int) {
	return os.checked, os.pos
}

// repairCheckedBytes makes sure that the checked.Bytes wraps the rawBuffer as
// they may have fallen out of sync during the writing process.
func (os *ostream) repairCheckedBytes() {
	if os.checked != nil {
		os.checked.Reset(os.rawBuffer)
	}
}

func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}
