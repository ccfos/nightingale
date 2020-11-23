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

package postings

import (
	"go.uber.org/atomic"
)

// AtomicID is an atomic ID.
type AtomicID struct {
	internal *atomic.Uint32
}

// NewAtomicID creates a new AtomicID.
func NewAtomicID(id ID) AtomicID {
	return AtomicID{internal: atomic.NewUint32(uint32(id))}
}

// Load atomically loads the ID.
func (id AtomicID) Load() ID {
	return ID(id.internal.Load())
}

// Inc atomically increments the ID and returns the new value.
func (id AtomicID) Inc() ID {
	return ID(id.internal.Inc())
}

// Add atomically adds n to the ID and returns the new ID.
func (id AtomicID) Add(n uint32) ID {
	return ID(id.internal.Add(n))
}
