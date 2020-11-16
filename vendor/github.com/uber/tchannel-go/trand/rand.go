// Copyright (c) 2015 Uber Technologies, Inc.

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

// Package trand provides a thread-safe random number generator.
package trand

import (
	"math/rand"
	"sync"
	"time"
)

// lockedSource allows a random number generator to be used by multiple goroutines
// concurrently. The code is very similar to math/rand.lockedSource, which is
// unfortunately not exposed.
type lockedSource struct {
	sync.Mutex

	src rand.Source
}

// New returns a rand.Rand that is threadsafe.
func New(seed int64) *rand.Rand {
	return rand.New(&lockedSource{src: rand.NewSource(seed)})
}

// NewSeeded returns a rand.Rand that's threadsafe and seeded with the current
// time.
func NewSeeded() *rand.Rand {
	return New(time.Now().UnixNano())
}

func (r *lockedSource) Int63() (n int64) {
	r.Lock()
	n = r.src.Int63()
	r.Unlock()
	return
}

func (r *lockedSource) Seed(seed int64) {
	r.Lock()
	r.src.Seed(seed)
	r.Unlock()
}
