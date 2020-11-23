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
	"hash"
	"hash/adler32"
	"os"

	xclose "github.com/m3db/m3/src/x/close"
)

// FdWithDigest is a container for a file descriptor and the digest for the file contents.
type FdWithDigest interface {
	xclose.Closer

	// Fd returns the file descriptor.
	Fd() *os.File

	// Digest returns the digest.
	Digest() hash.Hash32

	// Reset resets the file descriptor and the digest.
	Reset(fd *os.File)
}

type fdWithDigest struct {
	fd     *os.File
	digest hash.Hash32
}

func newFdWithDigest() FdWithDigest {
	return &fdWithDigest{
		digest: adler32.New(),
	}
}

func (fwd *fdWithDigest) Fd() *os.File {
	return fwd.fd
}

func (fwd *fdWithDigest) Digest() hash.Hash32 {
	return fwd.digest
}

func (fwd *fdWithDigest) Reset(fd *os.File) {
	fwd.fd = fd
	fwd.digest.Reset()
}

// Close closes the file descriptor.
func (fwd *fdWithDigest) Close() error {
	if fwd.fd == nil {
		return nil
	}
	err := fwd.fd.Close()
	fwd.fd = nil
	return err
}
