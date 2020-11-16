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

package x

import (
	"io"

	xerrors "github.com/m3db/m3/src/x/errors"
)

// NewSafeCloser returns a io.Closer which ensures the
// underlying Close() is only called once. It's
// useful for cleanup of resources in functions.
func NewSafeCloser(x io.Closer) io.Closer {
	return &safeCloser{Closer: x}
}

// safeCloser ensures Close() is only called once. It's
// useful for cleanup of resources in functions.
type safeCloser struct {
	io.Closer
	closed bool
}

// Close guarantees the underlying Closable's Close() is
// only executed the first time it's called.
func (c *safeCloser) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	return c.Closer.Close()
}

// NewSafeMultiCloser returns an io.Closer which ensures the that calling Close() on it
// calls Close() on each of the provided io.Closer(s) exactly once. Subsequent calls
// are not executed.
func NewSafeMultiCloser(closers ...io.Closer) io.Closer {
	return NewSafeCloser(ioClosers(closers))
}

type ioClosers []io.Closer

var _ io.Closer = ioClosers{}

func (ioc ioClosers) Close() error {
	var multiErr xerrors.MultiError
	for _, c := range ioc {
		multiErr = multiErr.Add(c.Close())
	}
	return multiErr.FinalError()
}
