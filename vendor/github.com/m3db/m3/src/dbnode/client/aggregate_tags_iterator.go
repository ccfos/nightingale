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

package client

import (
	"github.com/m3db/m3/src/x/ident"
)

// FOLLOWUP(r): add pooling for aggregateTagsIterator(s).
type aggregateTagsIterator struct {
	currentIdx int
	err        error
	pools      fetchTaggedPools

	current struct {
		tagName   ident.ID
		tagValues ident.Iterator
	}

	backing []*aggregateTagsIteratorTag
}

type aggregateTagsIteratorTag struct {
	tagName   ident.ID
	tagValues []ident.ID
}

// make the compiler ensure the concrete type `&aggregateTagsIterator{}` implements
// the `AggregatedTagsIterator` interface.
var _ AggregatedTagsIterator = &aggregateTagsIterator{}

func newAggregateTagsIterator(pools fetchTaggedPools) *aggregateTagsIterator {
	return &aggregateTagsIterator{
		currentIdx: -1,
		pools:      pools,
	}
}

func (i *aggregateTagsIterator) Next() bool {
	if i.err != nil || i.currentIdx >= len(i.backing) {
		return false
	}

	i.release()
	i.currentIdx++
	if i.currentIdx >= len(i.backing) {
		return false
	}

	i.current.tagName = i.backing[i.currentIdx].tagName
	i.current.tagValues = ident.NewIDSliceIterator(i.backing[i.currentIdx].tagValues)
	return true
}

func (i *aggregateTagsIterator) addTag(tagName []byte) *aggregateTagsIteratorTag {
	tag := &aggregateTagsIteratorTag{
		tagName: ident.BytesID(tagName),
	}
	i.backing = append(i.backing, tag)
	return tag
}

func (i *aggregateTagsIterator) release() {
	i.current.tagName = nil
	i.current.tagValues = nil
}

func (i *aggregateTagsIterator) Finalize() {
	i.release()
	for _, b := range i.backing {
		b.tagName.Finalize()
		for _, v := range b.tagValues {
			v.Finalize()
		}
	}
	i.backing = nil
}

func (i *aggregateTagsIterator) Remaining() int {
	return len(i.backing) - (i.currentIdx + 1)
}

func (i *aggregateTagsIterator) Current() (tagName ident.ID, tagValues ident.Iterator) {
	return i.current.tagName, i.current.tagValues
}

func (i *aggregateTagsIterator) Err() error {
	return i.err
}
