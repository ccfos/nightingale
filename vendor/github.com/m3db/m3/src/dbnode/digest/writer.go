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
	"io"
	"os"
)

// FdWithDigestWriter provides a buffered writer for writing to the underlying file.
type FdWithDigestWriter interface {
	FdWithDigest
	io.Writer

	Flush() error
}

type fdWithDigestWriter struct {
	FdWithDigest
	writer *bufio.Writer
}

// NewFdWithDigestWriter creates a new FdWithDigestWriter.
func NewFdWithDigestWriter(bufferSize int) FdWithDigestWriter {
	return &fdWithDigestWriter{
		FdWithDigest: newFdWithDigest(),
		writer:       bufio.NewWriterSize(nil, bufferSize),
	}
}

func (w *fdWithDigestWriter) Reset(fd *os.File) {
	w.FdWithDigest.Reset(fd)
	w.writer.Reset(fd)
}

// Write bytes to the underlying file.
func (w *fdWithDigestWriter) Write(b []byte) (int, error) {
	written, err := w.writer.Write(b)
	if err != nil {
		return 0, err
	}
	if _, err := w.FdWithDigest.Digest().Write(b); err != nil {
		return 0, err
	}
	return written, nil
}

// Close flushes what's remaining in the buffered writer and closes
// the underlying file.
func (w *fdWithDigestWriter) Close() error {
	if err := w.writer.Flush(); err != nil {
		return err
	}
	return w.FdWithDigest.Close()
}

// Flush flushes what's remaining in the buffered writes.
func (w *fdWithDigestWriter) Flush() error {
	return w.writer.Flush()
}

// FdWithDigestContentsWriter provides additional functionality of writing a digest to the underlying file.
type FdWithDigestContentsWriter interface {
	FdWithDigestWriter

	// WriteDigests writes a list of digests to the underlying file.
	WriteDigests(digests ...uint32) error
}

type fdWithDigestContentsWriter struct {
	FdWithDigestWriter

	digestBuf Buffer
}

// NewFdWithDigestContentsWriter creates a new FdWithDigestContentsWriter.
func NewFdWithDigestContentsWriter(bufferSize int) FdWithDigestContentsWriter {
	return &fdWithDigestContentsWriter{
		FdWithDigestWriter: NewFdWithDigestWriter(bufferSize),
		digestBuf:          NewBuffer(),
	}
}

func (w *fdWithDigestContentsWriter) WriteDigests(digests ...uint32) error {
	for _, digest := range digests {
		w.digestBuf.WriteDigest(digest)
		if _, err := w.Write(w.digestBuf); err != nil {
			return err
		}
	}
	return nil
}
