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

// Package checked implements reference counted resources.
package checked

import (
	"github.com/m3db/m3/src/x/resource"
)

// Ref is an entity that checks ref counts.
type Ref interface {
	// IncRef increments the ref count to this entity.
	IncRef()

	// DecRef decrements the ref count to this entity.
	DecRef()

	// MoveRef signals a move of the ref to this entity.
	MoveRef()

	// NumRef returns the ref count to this entity.
	NumRef() int

	// DelayFinalizer will delay calling the finalizer on this entity
	// until the closer returned by the method is called at least once.
	// This is useful for dependent resources requiring the lifetime of this
	// entityt to be extended.
	DelayFinalizer() resource.Closer

	// Finalize will call the finalizer if any, ref count must be zero.
	Finalize()
}

// OnFinalize is callback to cleanup resources on a call to finalize.
type OnFinalize interface {
	OnFinalize()
}

// OnFinalizeFn is a function literal that is a finalizer callback.
type OnFinalizeFn func()

// OnFinalize will call the function literal as a finalizer callback.
func (fn OnFinalizeFn) OnFinalize() {
	fn()
}

// Read is an entity that checks reads.
type Read interface {
	// IncReads increments the reads count to this entity.
	IncReads()

	// DecReads decrements the reads count to this entity.
	DecReads()

	// NumReaders returns the active reads count to this entity.
	NumReaders() int
}

// Write is an entity that checks writes.
type Write interface {
	// IncWrites increments the writes count to this entity.
	IncWrites()

	// DecWrites decrements the writes count to this entity.
	DecWrites()

	// NumWriters returns the active writes count to this entity.
	NumWriters() int
}

// ReadWriteRef is an entity that checks ref counts, reads and writes.
type ReadWriteRef interface {
	Ref
	Read
	Write
}

// BytesFinalizer finalizes a checked byte slice.
type BytesFinalizer interface {
	FinalizeBytes(b Bytes)
}

// BytesFinalizerFn is a function literal that is a bytes finalizer.
type BytesFinalizerFn func(b Bytes)

// FinalizeBytes will call the function literal as a bytes finalizer.
func (fn BytesFinalizerFn) FinalizeBytes(b Bytes) {
	fn(b)
}

// BytesOptions is a bytes option
type BytesOptions interface {
	// Finalizer is a bytes finalizer to call when finalized.
	Finalizer() BytesFinalizer

	// SetFinalizer sets a bytes finalizer to call when finalized.
	SetFinalizer(value BytesFinalizer) BytesOptions
}
