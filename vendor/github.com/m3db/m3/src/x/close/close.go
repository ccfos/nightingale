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

// Package close provides utilities for closing resources.
package close

import (
	"errors"
	"io"
)

var (
	// ErrNotCloseable is returned when trying to close a resource
	// that does not conform to a closeable interface.
	ErrNotCloseable = errors.New("not a closeable resource")
)

// Closer is a resource that can be closed.
type Closer interface {
	io.Closer
}

// CloserFn implements the SimpleCloser interface.
type CloserFn func() error

// Close implements the SimplerCloser interface.
func (fn CloserFn) Close() error {
	return fn()
}

// SimpleCloser is a resource that can be closed without returning a result.
type SimpleCloser interface {
	Close()
}

// SimpleCloserFn implements the SimpleCloser interface.
type SimpleCloserFn func()

// Close implements the SimplerCloser interface.
func (fn SimpleCloserFn) Close() {
	fn()
}

// TryClose attempts to close a resource, the resource is expected to
// implement either Closeable or CloseableResult.
func TryClose(r interface{}) error {
	if r, ok := r.(Closer); ok {
		return r.Close()
	}
	if r, ok := r.(SimpleCloser); ok {
		r.Close()
		return nil
	}
	return ErrNotCloseable
}
