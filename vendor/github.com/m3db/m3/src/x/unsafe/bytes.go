// Copyright (c) 2019 Uber Technologies, Inc.
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

// StringFn processes a byte slice.
type StringFn func(string)

// StringAndArgFn takes an argument alongside the byte slice.
type StringAndArgFn func(string, interface{})

// WithString converts a byte slice to a string with zero heap memory
// allocations, and calls a function to process the string. It is the caller's
// responsibility to make sure it holds no reference to the string after the
// function returns.
func WithString(b []byte, fn StringFn) {
	// NB(r): regardless of whether the backing array is allocated on the heap
	// or on the stack, it should still be valid before the byte slice goes out of scope
	// so it's safe to call the function on the underlying byte slice.
	fn(String(b))
}

// WithStringAndArg converts a byte slice to a string with zero heap memory
// allocations, and calls a function to process the string with one argument.
// It is the caller's responsibility to make sure it holds no reference to the
// string after the function returns.
func WithStringAndArg(b []byte, arg interface{}, fn StringAndArgFn) {
	fn(String(b), arg)
}

// String returns a string backed by a byte slice, it is the caller's
// responsibility not to mutate the bytes while using the string returned. It
// is much safer to use WithString and WithStringAndArg if possible, which is
// more likely to force use of the result to just a small block of code.
func String(b []byte) string {
	var s string
	if len(b) == 0 {
		return s
	}

	// NB(r): We need to declare a real string so internally the compiler
	// knows to use an unsafe.Pointer to keep track of the underlying memory so that
	// once the strings's array pointer is updated with the pointer to the byte slices's
	// underlying bytes, the compiler won't prematurely GC the memory when the byte slice
	// goes out of scope.
	stringHeader := (*reflect.StringHeader)(unsafe.Pointer(&s))

	// NB(r): This makes sure that even if GC relocates the byte slices's underlying
	// memory after this assignment, the corresponding unsafe.Pointer in the internal
	// string struct will be updated accordingly to reflect the memory relocation.
	stringHeader.Data = (*reflect.SliceHeader)(unsafe.Pointer(&b)).Data

	// NB(r): It is important that we access b after we assign the Data
	// pointer of the byte slice header to the Data pointer of the string header to
	// make sure the bytes don't get GC'ed before the assignment happens.
	l := len(b)
	stringHeader.Len = l

	return s
}
