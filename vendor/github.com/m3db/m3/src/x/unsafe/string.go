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

// Package unsafe contains operations that step around the type safety of Go programs.
package unsafe

import (
	"reflect"
	"unsafe"
)

// ImmutableBytes represents an immutable byte slice.
type ImmutableBytes []byte

// BytesFn processes a byte slice.
type BytesFn func(ImmutableBytes)

// BytesAndArgFn takes an argument alongside the byte slice.
type BytesAndArgFn func(ImmutableBytes, interface{})

// WithBytes converts a string to a byte slice with zero heap memory allocations,
// and calls a function to process the byte slice. It is the caller's responsibility
// to make sure the callback function passed in does not modify the byte slice
// in any way, and holds no reference to the byte slice after the function returns.
func WithBytes(s string, fn BytesFn) {
	// NB(xichen): Regardless of whether the backing array is allocated on the heap
	// or on the stack, it should still be valid before the string goes out of scope
	// so it's safe to call the function on the underlying byte slice.
	fn(Bytes(s))
}

// WithBytesAndArg converts a string to a byte slice with zero heap memory allocations,
// and calls a function to process the byte slice alongside one argument. It is the
// caller's responsibility to make sure the callback function passed in does not modify
// the byte slice in any way, and holds no reference to the byte slice after the function
// returns.
func WithBytesAndArg(s string, arg interface{}, fn BytesAndArgFn) {
	fn(Bytes(s), arg)
}

// Bytes returns the bytes backing a string, it is the caller's responsibility
// not to mutate the bytes returned. It is much safer to use WithBytes and
// WithBytesAndArg if possible, which is more likely to force use of the result
// to just a small block of code.
func Bytes(s string) ImmutableBytes {
	if len(s) == 0 {
		return nil
	}

	// NB(xichen): We need to declare a real byte slice so internally the compiler
	// knows to use an unsafe.Pointer to keep track of the underlying memory so that
	// once the slice's array pointer is updated with the pointer to the string's
	// underlying bytes, the compiler won't prematurely GC the memory when the string
	// goes out of scope.
	var b []byte
	byteHeader := (*reflect.SliceHeader)(unsafe.Pointer(&b))

	// NB(xichen): This makes sure that even if GC relocates the string's underlying
	// memory after this assignment, the corresponding unsafe.Pointer in the internal
	// slice struct will be updated accordingly to reflect the memory relocation.
	byteHeader.Data = (*reflect.StringHeader)(unsafe.Pointer(&s)).Data

	// NB(xichen): It is important that we access s after we assign the Data
	// pointer of the string header to the Data pointer of the slice header to
	// make sure the string (and the underlying bytes backing the string) don't get
	// GC'ed before the assignment happens.
	l := len(s)
	byteHeader.Len = l
	byteHeader.Cap = l

	return b
}
