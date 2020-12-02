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

package context

import "sync/atomic"

type cancellable struct {
	cancelled int32
}

// NewCancellable creates a new cancellable object
func NewCancellable() Cancellable {
	return &cancellable{}
}

func (c *cancellable) IsCancelled() bool { return atomic.LoadInt32(&c.cancelled) == 1 }
func (c *cancellable) Cancel()           { atomic.StoreInt32(&c.cancelled, 1) }
func (c *cancellable) Reset()            { atomic.StoreInt32(&c.cancelled, 0) }

// NoOpCancellable is a no-op cancellable
var noOpCancellable Cancellable = cancellableNoOp{}

type cancellableNoOp struct{}

// NewNoOpCanncellable returns a no-op cancellable
func NewNoOpCanncellable() Cancellable      { return noOpCancellable }
func (c cancellableNoOp) IsCancelled() bool { return false }
func (c cancellableNoOp) Cancel()           {}
func (c cancellableNoOp) Reset()            {}
