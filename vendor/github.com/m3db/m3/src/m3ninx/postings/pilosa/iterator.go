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

package pilosa

import (
	"github.com/m3db/m3/src/m3ninx/postings"
	"github.com/m3dbx/pilosa/roaring"
)

// NB: need to do this to find a path into our postings list which doesn't require every
// insert to grab a lock. Need to make a non thread-safe version of our api.
// FOLLOWUP(prateek): tracking this issue in https://github.com/m3db/m3ninx/issues/65

type iterator struct {
	iter    *roaring.Iterator
	current uint64
	hasNext bool
}

var _ postings.Iterator = &iterator{}

// NewIterator returns a postings.Iterator wrapping a pilosa roaring.Iterator.
func NewIterator(iter *roaring.Iterator) postings.Iterator {
	return &iterator{
		iter:    iter,
		hasNext: true,
	}
}

func (p *iterator) Next() bool {
	if !p.hasNext {
		return false
	}
	v, eof := p.iter.Next()
	p.current = v
	p.hasNext = !eof
	return p.hasNext
}

func (p *iterator) Current() postings.ID {
	return postings.ID(p.current)
}

func (p *iterator) Err() error {
	return nil
}

func (p *iterator) Close() error {
	p.iter = nil
	p.hasNext = false
	return nil
}
