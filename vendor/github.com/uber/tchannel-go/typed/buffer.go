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
	"errors"
	"io"
)

var (
	// ErrEOF is returned when trying to read past end of buffer
	ErrEOF = errors.New("buffer is too small")

	// ErrBufferFull is returned when trying to write past end of buffer
	ErrBufferFull = errors.New("no more room in buffer")

	// errStringTooLong is returned when writing a string with length larger
	// than the allows length limit. Intentionally not exported, in case we
	// want to add more context in future.
	errStringTooLong = errors.New("string is too long")
)

// A ReadBuffer is a wrapper around an underlying []byte with methods to read from
// that buffer in big-endian format.
type ReadBuffer struct {
	buffer    []byte
	remaining []byte
	err       error
}

// NewReadBuffer returns a ReadBuffer wrapping a byte slice
func NewReadBuffer(buffer []byte) *ReadBuffer {
	return &ReadBuffer{buffer: buffer, remaining: buffer}
}

// NewReadBufferWithSize returns a ReadBuffer with a given capacity
func NewReadBufferWithSize(size int) *ReadBuffer {
	return &ReadBuffer{buffer: make([]byte, size), remaining: nil}
}

// ReadSingleByte reads the next byte from the buffer
func (r *ReadBuffer) ReadSingleByte() byte {
	b, _ := r.ReadByte()
	return b
}

// ReadByte returns the next byte from the buffer.
func (r *ReadBuffer) ReadByte() (byte, error) {
	if r.err != nil {
		return 0, r.err
	}

	if len(r.remaining) < 1 {
		r.err = ErrEOF
		return 0, r.err
	}

	b := r.remaining[0]
	r.remaining = r.remaining[1:]
	return b, nil
}

// ReadBytes returns the next n bytes from the buffer
func (r *ReadBuffer) ReadBytes(n int) []byte {
	if r.err != nil {
		return nil
	}

	if len(r.remaining) < n {
		r.err = ErrEOF
		return nil
	}

	b := r.remaining[0:n]
	r.remaining = r.remaining[n:]
	return b
}

// ReadString returns a string of size n from the buffer
func (r *ReadBuffer) ReadString(n int) string {
	if b := r.ReadBytes(n); b != nil {
		// TODO(mmihic): This creates a copy, which sucks
		return string(b)
	}

	return ""
}

// ReadUint16 returns the next value in the buffer as a uint16
func (r *ReadBuffer) ReadUint16() uint16 {
	if b := r.ReadBytes(2); b != nil {
		return binary.BigEndian.Uint16(b)
	}

	return 0
}

// ReadUint32 returns the next value in the buffer as a uint32
func (r *ReadBuffer) ReadUint32() uint32 {
	if b := r.ReadBytes(4); b != nil {
		return binary.BigEndian.Uint32(b)
	}

	return 0
}

// ReadUint64 returns the next value in the buffer as a uint64
func (r *ReadBuffer) ReadUint64() uint64 {
	if b := r.ReadBytes(8); b != nil {
		return binary.BigEndian.Uint64(b)
	}

	return 0
}

// ReadUvarint reads an unsigned varint from the buffer.
func (r *ReadBuffer) ReadUvarint() uint64 {
	v, _ := binary.ReadUvarint(r)
	return v
}

// ReadLen8String reads an 8-bit length preceded string value
func (r *ReadBuffer) ReadLen8String() string {
	n := r.ReadSingleByte()
	return r.ReadString(int(n))
}

// ReadLen16String reads a 16-bit length preceded string value
func (r *ReadBuffer) ReadLen16String() string {
	n := r.ReadUint16()
	return r.ReadString(int(n))
}

// BytesRemaining returns the number of unconsumed bytes remaining in the buffer
func (r *ReadBuffer) BytesRemaining() int {
	return len(r.remaining)
}

// FillFrom fills the buffer from a reader
func (r *ReadBuffer) FillFrom(ior io.Reader, n int) (int, error) {
	if len(r.buffer) < n {
		return 0, ErrEOF
	}

	r.err = nil
	r.remaining = r.buffer[:n]
	return io.ReadFull(ior, r.remaining)
}

// Wrap initializes the buffer to read from the given byte slice
func (r *ReadBuffer) Wrap(b []byte) {
	r.buffer = b
	r.remaining = b
	r.err = nil
}

// Err returns the error in the ReadBuffer
func (r *ReadBuffer) Err() error { return r.err }

// A WriteBuffer is a wrapper around an underlying []byte with methods to write to
// that buffer in big-endian format.  The buffer is of fixed size, and does not grow.
type WriteBuffer struct {
	buffer    []byte
	remaining []byte
	err       error
}

// NewWriteBuffer creates a WriteBuffer wrapping the given slice
func NewWriteBuffer(buffer []byte) *WriteBuffer {
	return &WriteBuffer{buffer: buffer, remaining: buffer}
}

// NewWriteBufferWithSize create a new WriteBuffer using an internal buffer of the given size
func NewWriteBufferWithSize(size int) *WriteBuffer {
	return NewWriteBuffer(make([]byte, size))
}

// WriteSingleByte writes a single byte to the buffer
func (w *WriteBuffer) WriteSingleByte(n byte) {
	if w.err != nil {
		return
	}

	if len(w.remaining) == 0 {
		w.setErr(ErrBufferFull)
		return
	}

	w.remaining[0] = n
	w.remaining = w.remaining[1:]
}

// WriteBytes writes a slice of bytes to the buffer
func (w *WriteBuffer) WriteBytes(in []byte) {
	if b := w.reserve(len(in)); b != nil {
		copy(b, in)
	}
}

// WriteUint16 writes a big endian encoded uint16 value to the buffer
func (w *WriteBuffer) WriteUint16(n uint16) {
	if b := w.reserve(2); b != nil {
		binary.BigEndian.PutUint16(b, n)
	}
}

// WriteUint32 writes a big endian uint32 value to the buffer
func (w *WriteBuffer) WriteUint32(n uint32) {
	if b := w.reserve(4); b != nil {
		binary.BigEndian.PutUint32(b, n)
	}
}

// WriteUint64 writes a big endian uint64 to the buffer
func (w *WriteBuffer) WriteUint64(n uint64) {
	if b := w.reserve(8); b != nil {
		binary.BigEndian.PutUint64(b, n)
	}
}

// WriteUvarint writes an unsigned varint to the buffer
func (w *WriteBuffer) WriteUvarint(n uint64) {
	// A uvarint could be up to 10 bytes long.
	buf := make([]byte, 10)
	varBytes := binary.PutUvarint(buf, n)
	if b := w.reserve(varBytes); b != nil {
		copy(b, buf[0:varBytes])
	}
}

// WriteString writes a string to the buffer
func (w *WriteBuffer) WriteString(s string) {
	// NB(mmihic): Don't just call WriteBytes; that will make a double copy
	// of the string due to the cast
	if b := w.reserve(len(s)); b != nil {
		copy(b, s)
	}
}

// WriteLen8String writes an 8-bit length preceded string
func (w *WriteBuffer) WriteLen8String(s string) {
	if int(byte(len(s))) != len(s) {
		w.setErr(errStringTooLong)
	}

	w.WriteSingleByte(byte(len(s)))
	w.WriteString(s)
}

// WriteLen16String writes a 16-bit length preceded string
func (w *WriteBuffer) WriteLen16String(s string) {
	if int(uint16(len(s))) != len(s) {
		w.setErr(errStringTooLong)
	}

	w.WriteUint16(uint16(len(s)))
	w.WriteString(s)
}

// DeferByte reserves space in the buffer for a single byte, and returns a
// reference that can be used to update that byte later
func (w *WriteBuffer) DeferByte() ByteRef {
	if len(w.remaining) == 0 {
		w.setErr(ErrBufferFull)
		return ByteRef(nil)
	}

	// Always zero out references, since the caller expects the default to be 0.
	w.remaining[0] = 0
	bufRef := ByteRef(w.remaining[0:])
	w.remaining = w.remaining[1:]
	return bufRef
}

// DeferUint16 reserves space in the buffer for a uint16, and returns a
// reference that can be used to update that uint16
func (w *WriteBuffer) DeferUint16() Uint16Ref {
	return Uint16Ref(w.deferred(2))
}

// DeferUint32 reserves space in the buffer for a uint32, and returns a
// reference that can be used to update that uint32
func (w *WriteBuffer) DeferUint32() Uint32Ref {
	return Uint32Ref(w.deferred(4))
}

// DeferUint64 reserves space in the buffer for a uint64, and returns a
// reference that can be used to update that uint64
func (w *WriteBuffer) DeferUint64() Uint64Ref {
	return Uint64Ref(w.deferred(8))
}

// DeferBytes reserves space in the buffer for a fixed sequence of bytes, and
// returns a reference that can be used to update those bytes
func (w *WriteBuffer) DeferBytes(n int) BytesRef {
	return BytesRef(w.deferred(n))
}

func (w *WriteBuffer) deferred(n int) []byte {
	bs := w.reserve(n)
	for i := range bs {
		bs[i] = 0
	}
	return bs
}

func (w *WriteBuffer) reserve(n int) []byte {
	if w.err != nil {
		return nil
	}

	if len(w.remaining) < n {
		w.setErr(ErrBufferFull)
		return nil
	}

	b := w.remaining[0:n]
	w.remaining = w.remaining[n:]
	return b
}

// BytesRemaining returns the number of available bytes remaining in the bufffer
func (w *WriteBuffer) BytesRemaining() int {
	return len(w.remaining)
}

// FlushTo flushes the written buffer to the given writer
func (w *WriteBuffer) FlushTo(iow io.Writer) (int, error) {
	dirty := w.buffer[0:w.BytesWritten()]
	return iow.Write(dirty)
}

// BytesWritten returns the number of bytes that have been written to the buffer
func (w *WriteBuffer) BytesWritten() int { return len(w.buffer) - len(w.remaining) }

// Reset resets the buffer to an empty state, ready for writing
func (w *WriteBuffer) Reset() {
	w.remaining = w.buffer
	w.err = nil
}

func (w *WriteBuffer) setErr(err error) {
	// Only store the first error
	if w.err != nil {
		return
	}

	w.err = err
}

// Err returns the current error in the buffer
func (w *WriteBuffer) Err() error { return w.err }

// Wrap initializes the buffer to wrap the given byte slice
func (w *WriteBuffer) Wrap(b []byte) {
	w.buffer = b
	w.remaining = b
}

// A ByteRef is a reference to a byte in a bufffer
type ByteRef []byte

// Update updates the byte in the buffer
func (ref ByteRef) Update(b byte) {
	if ref != nil {
		ref[0] = b
	}
}

// A Uint16Ref is a reference to a uint16 placeholder in a buffer
type Uint16Ref []byte

// Update updates the uint16 in the buffer
func (ref Uint16Ref) Update(n uint16) {
	if ref != nil {
		binary.BigEndian.PutUint16(ref, n)
	}
}

// A Uint32Ref is a reference to a uint32 placeholder in a buffer
type Uint32Ref []byte

// Update updates the uint32 in the buffer
func (ref Uint32Ref) Update(n uint32) {
	if ref != nil {
		binary.BigEndian.PutUint32(ref, n)
	}
}

// A Uint64Ref is a reference to a uin64 placeholder in a buffer
type Uint64Ref []byte

// Update updates the uint64 in the buffer
func (ref Uint64Ref) Update(n uint64) {
	if ref != nil {
		binary.BigEndian.PutUint64(ref, n)
	}
}

// A BytesRef is a reference to a multi-byte placeholder in a buffer
type BytesRef []byte

// Update updates the bytes in the buffer
func (ref BytesRef) Update(b []byte) {
	if ref != nil {
		copy(ref, b)
	}
}

// UpdateString updates the bytes in the buffer from a string
func (ref BytesRef) UpdateString(s string) {
	if ref != nil {
		copy(ref, s)
	}
}
