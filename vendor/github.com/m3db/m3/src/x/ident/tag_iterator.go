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

package ident

import (
	"errors"

	"github.com/m3db/m3/src/m3ninx/doc"
)

var (
	errInvalidNumberInputsToIteratorMatcher = errors.New("inputs must be specified in name-value pairs (i.e. divisible by 2)")
)

// MustNewTagStringsIterator returns a TagIterator over a slice of strings,
// panic'ing if it encounters an error.
func MustNewTagStringsIterator(inputs ...string) TagIterator {
	iter, err := NewTagStringsIterator(inputs...)
	if err != nil {
		panic(err.Error())
	}
	return iter
}

// NewTagStringsIterator returns a TagIterator over a slice of strings.
func NewTagStringsIterator(inputs ...string) (TagIterator, error) {
	if len(inputs)%2 != 0 {
		return nil, errInvalidNumberInputsToIteratorMatcher
	}
	tags := make([]Tag, 0, len(inputs)/2)
	for i := 0; i < len(inputs); i += 2 {
		tags = append(tags, StringTag(inputs[i], inputs[i+1]))
	}
	return NewTagsIterator(NewTags(tags...)), nil
}

// NewTagsIterator returns a TagsIterator over a set of tags.
func NewTagsIterator(tags Tags) TagsIterator {
	return newTagSliceIter(tags, nil, nil)
}

// NewFieldsTagsIterator returns a TagsIterator over a set of fields.
func NewFieldsTagsIterator(fields []doc.Field) TagsIterator {
	return newTagSliceIter(Tags{}, fields, nil)
}

func newTagSliceIter(
	tags Tags,
	fields []doc.Field,
	pool Pool,
) *tagSliceIter {
	iter := &tagSliceIter{
		nameBytesID:  NewReuseableBytesID(),
		valueBytesID: NewReuseableBytesID(),
		pool:         pool,
	}
	iter.currentReuseableTag = Tag{
		Name:  iter.nameBytesID,
		Value: iter.valueBytesID,
	}
	if len(tags.Values()) > 0 {
		iter.Reset(tags)
	} else {
		iter.ResetFields(fields)
	}
	return iter
}

type tagsSliceType uint

const (
	tagSliceType tagsSliceType = iota
	fieldSliceType
)

type tagsSlice struct {
	tags   []Tag
	fields []doc.Field
}

type tagSliceIter struct {
	backingSlice        tagsSlice
	currentIdx          int
	currentTag          Tag
	currentReuseableTag Tag
	nameBytesID         *ReuseableBytesID
	valueBytesID        *ReuseableBytesID
	pool                Pool
}

func (i *tagSliceIter) Next() bool {
	i.currentIdx++
	l, t := i.lengthAndType()
	if i.currentIdx < l {
		if t == tagSliceType {
			i.currentTag = i.backingSlice.tags[i.currentIdx]
		} else {
			i.nameBytesID.Reset(i.backingSlice.fields[i.currentIdx].Name)
			i.valueBytesID.Reset(i.backingSlice.fields[i.currentIdx].Value)
			i.currentTag = i.currentReuseableTag
		}
		return true
	}
	i.currentTag = Tag{}
	return false
}

func (i *tagSliceIter) Current() Tag {
	return i.currentTag
}

func (i *tagSliceIter) CurrentIndex() int {
	if i.currentIdx >= 0 {
		return i.currentIdx
	}
	return 0
}

func (i *tagSliceIter) Err() error {
	return nil
}

func (i *tagSliceIter) Close() {
	i.backingSlice = tagsSlice{}
	i.currentIdx = 0
	i.currentTag = Tag{}

	if i.pool == nil {
		return
	}

	i.pool.PutTagsIterator(i)
}

func (i *tagSliceIter) Len() int {
	l, _ := i.lengthAndType()
	return l
}

func (i *tagSliceIter) lengthAndType() (int, tagsSliceType) {
	if l := len(i.backingSlice.tags); l > 0 {
		return l, tagSliceType
	}
	return len(i.backingSlice.fields), fieldSliceType
}

func (i *tagSliceIter) Remaining() int {
	if r := i.Len() - 1 - i.currentIdx; r >= 0 {
		return r
	}
	return 0
}

func (i *tagSliceIter) Duplicate() TagIterator {
	if i.pool != nil {
		iter := i.pool.TagsIterator()
		if len(i.backingSlice.tags) > 0 {
			iter.Reset(Tags{values: i.backingSlice.tags})
		} else {
			iter.ResetFields(i.backingSlice.fields)
		}
		for j := 0; j <= i.currentIdx; j++ {
			iter.Next()
		}
		return iter
	}
	return newTagSliceIter(Tags{values: i.backingSlice.tags},
		i.backingSlice.fields, i.pool)
}

func (i *tagSliceIter) rewind() {
	i.currentIdx = -1
	i.currentTag = Tag{}
}

func (i *tagSliceIter) Reset(tags Tags) {
	i.backingSlice = tagsSlice{tags: tags.Values()}
	i.rewind()
}

func (i *tagSliceIter) ResetFields(fields []doc.Field) {
	i.backingSlice = tagsSlice{fields: fields}
	i.rewind()
}

func (i *tagSliceIter) Rewind() {
	i.rewind()
}

// EmptyTagIterator returns an iterator over no tags.
var EmptyTagIterator TagIterator = emptyTagIterator{}

type emptyTagIterator struct{}

func (e emptyTagIterator) Next() bool             { return false }
func (e emptyTagIterator) Current() Tag           { return Tag{} }
func (e emptyTagIterator) CurrentIndex() int      { return 0 }
func (e emptyTagIterator) Err() error             { return nil }
func (e emptyTagIterator) Close()                 {}
func (e emptyTagIterator) Len() int               { return 0 }
func (e emptyTagIterator) Remaining() int         { return 0 }
func (e emptyTagIterator) Duplicate() TagIterator { return e }
func (e emptyTagIterator) Rewind()                {}
