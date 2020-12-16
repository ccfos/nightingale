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

// Package ident provides utilities for working with identifiers.
package ident

import (
	"fmt"

	"github.com/m3db/m3/src/m3ninx/doc"
	"github.com/m3db/m3/src/x/checked"
	"github.com/m3db/m3/src/x/context"
)

// ID represents an immutable identifier to allow use of byte slice pooling
// for the contents of the ID.
type ID interface {
	fmt.Stringer

	// Bytes returns the underlying byte slice of the bytes ID unpacked from
	// any checked bytes container, callers cannot safely hold a ref to these
	// bytes.
	Bytes() []byte

	// Equal returns whether the ID is equal to a given ID.
	Equal(value ID) bool

	// NoFinalize makes calls to finalize a no-op, this is useful when you
	// would like to share a type with another sub-system that should is not
	// allowed to finalize the resource as the resource is kept indefinitely
	// until garbage collected (i.e. longly lived).
	NoFinalize()

	// IsNoFinalize returns whether finalize is a no-op or not, this is useful
	// when you know you can use an ID without having to worry to take a copy.
	IsNoFinalize() bool

	// Finalize releases all resources held by the ID, unless NoFinalize has
	// been called previously in which case this is a no-op.
	Finalize()
}

// TagName represents the name of a timeseries tag.
type TagName ID

// TagValue represents the value of a timeseries tag.
type TagValue ID

// Tag represents a timeseries tag.
type Tag struct {
	Name       TagName
	Value      TagValue
	noFinalize bool
}

// NoFinalize makes calls to finalize a no-op, this is useful when you
// would like to share a type with another sub-system that should is not
// allowed to finalize the resource as the resource is kept indefinitely
// until garbage collected (i.e. longly lived).
func (t *Tag) NoFinalize() {
	t.noFinalize = true
	t.Name.NoFinalize()
	t.Value.NoFinalize()
}

// Finalize releases all resources held by the Tag, unless NoFinalize has
// been called previously in which case this is a no-op.
func (t *Tag) Finalize() {
	if t.noFinalize {
		return
	}
	if t.Name != nil {
		t.Name.Finalize()
		t.Name = nil
	}
	if t.Value != nil {
		t.Value.Finalize()
		t.Value = nil
	}
}

// Equal returns whether the two tags are equal.
func (t Tag) Equal(value Tag) bool {
	return t.Name.Equal(value.Name) && t.Value.Equal(value.Value)
}

// Pool represents an automatic pool of `ident` objects.
type Pool interface {
	// GetBinaryID will create a new binary ID and take reference to the bytes.
	// When the context closes the ID will be finalized and so too will
	// the bytes, i.e. it will take ownership of the bytes.
	GetBinaryID(c context.Context, data checked.Bytes) ID

	// BinaryID will create a new binary ID and take a reference to the bytes.
	BinaryID(data checked.Bytes) ID

	// GetBinaryTag will create a new binary Tag and take reference to the bytes.
	// When the context closes, the Tag will be finalized and so too will
	// the bytes, i.e. it will take ownership of the bytes.
	GetBinaryTag(c context.Context, name, value checked.Bytes) Tag

	// BinaryTag will create a new binary Tag and take a reference to the provided bytes.
	BinaryTag(name, value checked.Bytes) Tag

	// GetStringID will create a new string ID and create a bytes copy of the
	// string. When the context closes the ID will be finalized.
	GetStringID(c context.Context, id string) ID

	// StringID will create a new string ID and create a bytes copy of the
	// string.
	StringID(data string) ID

	// GetStringTag will create a new string Tag and create a bytes copy of the
	// string. When the context closes the ID will be finalized.
	GetStringTag(c context.Context, name, value string) Tag

	// StringTag will create a new string Tag and create a bytes copy of the
	// string.
	StringTag(name, value string) Tag

	// Tags will create a new array of tags and return it.
	Tags() Tags

	// GetTagsIterator will create a tag iterator and return it. When the context
	// closes the tags array and any tags contained will be finalized.
	GetTagsIterator(c context.Context) TagsIterator

	// TagsIterator will create a tag iterator and return it.
	TagsIterator() TagsIterator

	// Put an ID back in the pool.
	Put(id ID)

	// PutTag puts a tag back in the pool.
	PutTag(tag Tag)

	// PutTags puts a set of tags back in the pool.
	PutTags(tags Tags)

	// PutTagsIterator puts a tags iterator back in the pool.
	PutTagsIterator(iter TagsIterator)

	// Clone replicates a given ID into a pooled ID.
	Clone(id ID) ID

	// CloneTag replicates a given Tag into a pooled Tag.
	CloneTag(tag Tag) Tag

	// CloneTags replicates a given set of Tags into a pooled Tags.
	CloneTags(tags Tags) Tags
}

// Iterator represents an iterator over `ID` instances. It is not thread-safe.
type Iterator interface {
	// Next returns a bool indicating the presence of the next ID instance.
	Next() bool

	// Current returns the current ID instance.
	Current() ID

	// CurrentIndex returns the current index at.
	CurrentIndex() int

	// Close releases any resources held by the iterator.
	Close()

	// Err returns any errors encountered during iteration.
	Err() error

	// Len returns the number of elements.
	Len() int

	// Remaining returns the number of elements remaining to be iterated over.
	Remaining() int

	// Dupe returns an independent duplicate of the iterator.
	Duplicate() Iterator
}

// TagIterator represents an iterator over `Tag` instances. It is not thread-safe.
type TagIterator interface {
	// Next returns a bool indicating the presence of the next Tag instance.
	Next() bool

	// Current returns the current Tag instance.
	Current() Tag

	// CurrentIndex returns the current index at.
	CurrentIndex() int

	// Err returns any errors encountered during iteration.
	Err() error

	// Close releases any resources held by the iterator.
	Close()

	// Len returns the number of elements.
	Len() int

	// Remaining returns the number of elements remaining to be iterated over.
	Remaining() int

	// Duplicate returns an independent duplicate of the iterator.
	Duplicate() TagIterator

	// Rewind resets the tag iterator to the initial position.
	Rewind()
}

// TagsIterator represents a TagIterator that can be reset with a Tags
// collection type. It is not thread-safe.
type TagsIterator interface {
	TagIterator

	// Reset allows the tag iterator to be reused with a new set of tags.
	Reset(tags Tags)

	// ResetFields allows tag iterator to be reused from a set of fields.
	ResetFields(fields []doc.Field)
}

// Tags is a collection of Tag instances that can be pooled.
type Tags struct {
	values     []Tag
	pool       Pool
	noFinalize bool
}

// NewTags returns a new set of tags.
func NewTags(values ...Tag) Tags {
	return Tags{values: values}
}

// Reset resets the tags for reuse.
func (t *Tags) Reset(values []Tag) {
	t.values = values
}

// Values returns the tags values.
func (t Tags) Values() []Tag {
	return t.values
}

// Append will append a tag.
func (t *Tags) Append(tag Tag) {
	t.values = append(t.values, tag)
}

// NoFinalize makes calls to finalize a no-op, this is useful when you
// would like to share a type with another sub-system that should is not
// allowed to finalize the resource as the resource is kept indefinitely
// until garbage collected (i.e. longly lived).
func (t *Tags) NoFinalize() {
	t.noFinalize = true
	for _, tag := range t.values {
		tag.NoFinalize()
	}
}

// Finalize finalizes all Tags, unless NoFinalize has been called previously
// in which case this is a no-op.
func (t *Tags) Finalize() {
	if t.noFinalize {
		return
	}

	values := t.values
	t.values = nil

	for i := range values {
		values[i].Finalize()
	}

	if t.pool == nil {
		return
	}

	t.pool.PutTags(Tags{values: values})
}

// Equal returns a bool indicating if the tags are equal. It requires
// the two slices are ordered the same.
func (t Tags) Equal(other Tags) bool {
	if len(t.Values()) != len(other.Values()) {
		return false
	}
	for i := 0; i < len(t.Values()); i++ {
		equal := t.values[i].Name.Equal(other.values[i].Name) &&
			t.values[i].Value.Equal(other.values[i].Value)
		if !equal {
			return false
		}
	}
	return true
}
