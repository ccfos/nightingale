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

package builder

import (
	"runtime"

	"github.com/m3db/m3/src/m3ninx/postings"
	"github.com/m3db/m3/src/m3ninx/postings/roaring"
	"github.com/m3db/m3/src/m3ninx/util"
)

const (
	defaultInitialCapacity = 128
)

var (
	defaultConcurrency = runtime.NumCPU()
)

// Options is a collection of options for segment building.
type Options interface {
	// SetNewUUIDFn sets the function used to generate new UUIDs.
	SetNewUUIDFn(value util.NewUUIDFn) Options

	// NewUUIDFn returns the function used to generate new UUIDs.
	NewUUIDFn() util.NewUUIDFn

	// SetInitialCapacity sets the initial capacity.
	SetInitialCapacity(value int) Options

	// InitialCapacity returns the initial capacity.
	InitialCapacity() int

	// SetPostingsListPool sets the postings list pool.
	SetPostingsListPool(value postings.Pool) Options

	// PostingsListPool returns the postings list pool.
	PostingsListPool() postings.Pool

	// SetConcurrency sets the indexing concurrency.
	SetConcurrency(value int) Options

	// Concurrency returns the indexing concurrency.
	Concurrency() int
}

type opts struct {
	newUUIDFn       util.NewUUIDFn
	initialCapacity int
	postingsPool    postings.Pool
	concurrency     int
}

// NewOptions returns new options.
func NewOptions() Options {
	return &opts{
		newUUIDFn:       util.NewUUID,
		initialCapacity: defaultInitialCapacity,
		postingsPool:    postings.NewPool(nil, roaring.NewPostingsList),
		concurrency:     defaultConcurrency,
	}
}

func (o *opts) SetNewUUIDFn(v util.NewUUIDFn) Options {
	opts := *o
	opts.newUUIDFn = v
	return &opts
}

func (o *opts) NewUUIDFn() util.NewUUIDFn {
	return o.newUUIDFn
}

func (o *opts) SetInitialCapacity(v int) Options {
	opts := *o
	opts.initialCapacity = v
	return &opts
}

func (o *opts) InitialCapacity() int {
	return o.initialCapacity
}

func (o *opts) SetPostingsListPool(v postings.Pool) Options {
	opts := *o
	opts.postingsPool = v
	return &opts
}

func (o *opts) PostingsListPool() postings.Pool {
	return o.postingsPool
}

func (o *opts) SetConcurrency(v int) Options {
	opts := *o
	opts.concurrency = v
	return &opts
}

func (o *opts) Concurrency() int {
	return o.concurrency
}
