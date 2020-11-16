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

// Package resource describes require for object lifecycle management.
// Both Finalizer and Closer have similar concepts, they both exist so that
// different types can be used for resource cleanup with different method names
// as for some things like iterators, the verb close makes more sense than
// finalize and is more consistent with other types.
package resource

// Finalizer finalizes a checked resource.
type Finalizer interface {
	Finalize()
}

// FinalizerFn is a function literal that is a finalizer.
type FinalizerFn func()

// Finalize will call the function literal as a finalizer.
func (fn FinalizerFn) Finalize() {
	fn()
}

// Closer is an object that can be closed.
type Closer interface {
	Close()
}

// CloserFn is a function literal that is a closer.
type CloserFn func()

// Close will call the function literal as a closer.
func (fn CloserFn) Close() {
	fn()
}
