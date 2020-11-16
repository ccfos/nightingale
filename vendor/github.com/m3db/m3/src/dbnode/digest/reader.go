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
	"bufio"
	"errors"
	"hash"
	"hash/adler32"
	"io"
	"os"
)

var (
	// errReadFewerThanExpectedBytes returned when number of bytes read is fewer than expected
	errReadFewerThanExpectedBytes = errors.New("number of bytes read is fewer than expected")

	// errChecksumMismatch returned when the calculated checksum doesn't match the stored checksum
	errChecksumMismatch = errors.New("calculated checksum doesn't match stored checksum")

	// errBufferSizeMismatch returned when ReadAllAndValidate called without well sized buffer
	errBufferSizeMismatch = errors.New("buffer passed is not an exact fit for contents")
)

// FdWithDigestReader provides a buffered reader for reading from the underlying file.
type FdWithDigestReader interface {
	FdWithDigest
	io.Reader

	// ReadAllAndValidate reads everything in the underlying file and validates
	// it against the expected digest, returning an error if they don't match.
	// Note: the buffer "b" must be an exact match for how long the contents being
	// read is, the signature is structured this way to allow for buffer reuse.
	ReadAllAndValidate(b []byte, expectedDigest uint32) (int, error)

	// Validate compares the current digest against the expected digest and returns
	// an error if they don't match.
	Validate(expectedDigest uint32) error
}

type fdWithDigestReader struct {
	fd               *os.File
	bufReader        *bufio.Reader
	readerWithDigest ReaderWithDigest
	single           [1]byte
}

// NewFdWithDigestReader creates a new FdWithDigestReader.
func NewFdWithDigestReader(bufferSize int) FdWithDigestReader {
	bufReader := bufio.NewReaderSize(nil, bufferSize)
	return &fdWithDigestReader{
		bufReader:        bufReader,
		readerWithDigest: NewReaderWithDigest(bufReader),
	}
}

// Reset resets the underlying file descriptor and the buffered reader.
func (r *fdWithDigestReader) Reset(fd *os.File) {
	r.fd = fd
	r.bufReader.Reset(fd)
	r.readerWithDigest.Reset(r.bufReader)
}

func (r *fdWithDigestReader) Read(b []byte) (int, error) {
	return r.readerWithDigest.Read(b)
}

func (r *fdWithDigestReader) Fd() *os.File {
	return r.fd
}

func (r *fdWithDigestReader) Digest() hash.Hash32 {
	return r.readerWithDigest.Digest()
}

func (r *fdWithDigestReader) ReadAllAndValidate(b []byte, expectedDigest uint32) (int, error) {
	n, err := r.Read(b)
	if err != nil {
		return n, err
	}
	// NB(r): Attempt next read to prove that the size of the buffer b
	// was sized correctly to fit all contents into it and that we are
	// correctly now at the end of input.
	_, err = r.Read(r.single[:])
	if err != io.EOF {
		return 0, errBufferSizeMismatch
	}
	if err := r.Validate(expectedDigest); err != nil {
		return n, err
	}
	return n, nil
}

func (r *fdWithDigestReader) Validate(expectedDigest uint32) error {
	return r.readerWithDigest.Validate(expectedDigest)
}

func (r *fdWithDigestReader) Close() error {
	if r.fd == nil {
		return nil
	}
	err := r.fd.Close()
	r.fd = nil
	return err
}

// FdWithDigestContentsReader provides additional functionality of reading a digest from the underlying file.
type FdWithDigestContentsReader interface {
	FdWithDigestReader

	// ReadDigest reads a digest from the underlying file.
	ReadDigest() (uint32, error)
}

type fdWithDigestContentsReader struct {
	FdWithDigestReader

	digestBuf Buffer
}

// NewFdWithDigestContentsReader creates a new FdWithDigestContentsReader.
func NewFdWithDigestContentsReader(bufferSize int) FdWithDigestContentsReader {
	return &fdWithDigestContentsReader{
		FdWithDigestReader: NewFdWithDigestReader(bufferSize),
		digestBuf:          NewBuffer(),
	}
}

func (r *fdWithDigestContentsReader) ReadDigest() (uint32, error) {
	n, err := r.Read(r.digestBuf)
	if err != nil {
		return 0, err
	}
	if n < len(r.digestBuf) {
		return 0, errReadFewerThanExpectedBytes
	}
	return r.digestBuf.ReadDigest(), nil
}

// ReaderWithDigest is a reader that that calculates a digest
// as it is read.
type ReaderWithDigest interface {
	io.Reader

	// Reset resets the reader for use with a new reader.
	Reset(reader io.Reader)

	// Digest returns the digest.
	Digest() hash.Hash32

	// Validate compares the current digest against the expected digest and returns
	// an error if they don't match.
	Validate(expectedDigest uint32) error
}

type readerWithDigest struct {
	reader io.Reader
	digest hash.Hash32
}

// NewReaderWithDigest creates a new reader that calculates a digest as it
// reads an input.
func NewReaderWithDigest(reader io.Reader) ReaderWithDigest {
	return &readerWithDigest{
		reader: reader,
		digest: adler32.New(),
	}
}

func (r *readerWithDigest) Reset(reader io.Reader) {
	r.reader = reader
	r.digest.Reset()
}

func (r *readerWithDigest) Digest() hash.Hash32 {
	return r.digest
}

func (r *readerWithDigest) readBytes(b []byte) (int, error) {
	n, err := r.reader.Read(b)
	if err != nil {
		return 0, err
	}
	// In case the buffered reader only returns what's remaining in
	// the buffer, recursively read what's left in the underlying reader.
	if n < len(b) {
		b = b[n:]
		remainder, err := r.readBytes(b)
		return n + remainder, err
	}
	return n, err
}

func (r *readerWithDigest) Read(b []byte) (int, error) {
	n, err := r.readBytes(b)
	if err != nil && err != io.EOF {
		return n, err
	}
	// If we encountered an EOF error and didn't read any bytes
	// given a non-empty slice, we return an EOF error.
	if err == io.EOF && n == 0 && len(b) > 0 {
		return 0, err
	}
	if _, err := r.digest.Write(b[:n]); err != nil {
		return 0, err
	}
	return n, nil
}

func (r *readerWithDigest) Validate(expectedDigest uint32) error {
	if r.digest.Sum32() != expectedDigest {
		return errChecksumMismatch
	}
	return nil
}
