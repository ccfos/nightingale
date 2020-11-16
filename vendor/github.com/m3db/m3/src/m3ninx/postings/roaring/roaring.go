// Copyright (c) 2017 Uber Technologies, Inc.
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

package roaring

import (
	"errors"
	"fmt"

	"github.com/m3db/m3/src/m3ninx/postings"
	"github.com/m3db/m3/src/m3ninx/x"
	"github.com/m3dbx/pilosa/roaring"
)

var (
	errIntersectRoaringOnly  = errors.New("Intersect only supported between roaringDocId sets")
	errUnionRoaringOnly      = errors.New("Union only supported between roaringDocId sets")
	errDifferenceRoaringOnly = errors.New("Difference only supported between roaringDocId sets")
	errIteratorClosed        = errors.New("iterator has been closed")
)

// Union retrieves a new postings list which is the union of the provided lists.
func Union(inputs []postings.List) (postings.MutableList, error) {
	if len(inputs) == 0 {
		return NewPostingsList(), nil
	}

	unioned := roaring.NewBitmap()
	if err := union(unioned, inputs); err != nil {
		return nil, err
	}
	return NewPostingsListFromBitmap(unioned), nil
}

func union(unionedBitmap *roaring.Bitmap, inputs []postings.List) error {
	bitmaps := make([]*roaring.Bitmap, 0, len(inputs))
	for _, in := range inputs {
		pl, ok := in.(*postingsList)
		if !ok {
			return fmt.Errorf("unable to convert inputs into roaring postings lists")
		}
		bitmaps = append(bitmaps, pl.bitmap)
	}

	unionedBitmap.UnionInPlace(bitmaps...)
	return nil
}

// BitmapFromPostingsList returns a bitmap from a postings list if it
// is a roaring bitmap postings list.
func BitmapFromPostingsList(pl postings.List) (*roaring.Bitmap, bool) {
	result, ok := pl.(*postingsList)
	if !ok {
		return nil, false
	}
	return result.bitmap, true
}

// postingsList abstracts a Roaring Bitmap.
type postingsList struct {
	bitmap *roaring.Bitmap
}

// NewPostingsList returns a new mutable postings list backed by a Roaring Bitmap.
func NewPostingsList() postings.MutableList {
	return &postingsList{
		bitmap: roaring.NewBitmap(),
	}
}

// NewPostingsListFromBitmap returns a new mutable postings list using an
// existing roaring bitmap.
func NewPostingsListFromBitmap(bitmap *roaring.Bitmap) postings.MutableList {
	return &postingsList{bitmap: bitmap}
}

func (d *postingsList) Insert(i postings.ID) error {
	_ = d.bitmap.DirectAdd(uint64(i))
	return nil
}

func (d *postingsList) Intersect(other postings.List) error {
	o, ok := other.(*postingsList)
	if !ok {
		return errIntersectRoaringOnly
	}

	d.bitmap = d.bitmap.Intersect(o.bitmap)
	return nil
}

func (d *postingsList) Difference(other postings.List) error {
	o, ok := other.(*postingsList)
	if !ok {
		return errDifferenceRoaringOnly
	}

	d.bitmap = d.bitmap.Difference(o.bitmap)
	return nil
}

func (d *postingsList) Union(other postings.List) error {
	o, ok := other.(*postingsList)
	if !ok {
		return errUnionRoaringOnly
	}

	d.bitmap.UnionInPlace(o.bitmap)
	return nil
}

func (d *postingsList) UnionMany(others []postings.List) error {
	return union(d.bitmap, others)
}

func (d *postingsList) AddRange(min, max postings.ID) error {
	for i := min; i < max; i++ {
		_, err := d.bitmap.Add(uint64(i))
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *postingsList) AddIterator(iter postings.Iterator) error {
	safeIter := x.NewSafeCloser(iter)
	defer safeIter.Close()

	for iter.Next() {
		if err := d.Insert(iter.Current()); err != nil {
			return err
		}
	}

	if err := iter.Err(); err != nil {
		return err
	}

	return safeIter.Close()
}

func (d *postingsList) RemoveRange(min, max postings.ID) error {
	for i := min; i < max; i++ {
		_, err := d.bitmap.Remove(uint64(i))
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *postingsList) Reset() {
	d.bitmap.Reset()
}

func (d *postingsList) Contains(i postings.ID) bool {
	return d.bitmap.Contains(uint64(i))
}

func (d *postingsList) IsEmpty() bool {
	return d.bitmap.Count() == 0
}

func (d *postingsList) Max() (postings.ID, error) {
	if d.IsEmpty() {
		return 0, postings.ErrEmptyList
	}
	max := d.bitmap.Max()
	return postings.ID(max), nil
}

func (d *postingsList) Len() int {
	return int(d.bitmap.Count())
}

func (d *postingsList) Iterator() postings.Iterator {
	return &roaringIterator{
		iter: d.bitmap.Iterator(),
	}
}

func (d *postingsList) Clone() postings.MutableList {
	// TODO: It's cheaper to Clone than to cache roaring bitmaps, see
	// `postings_list_bench_test.go`. Their internals don't allow for
	// pooling at the moment. We should address this when get a chance
	// (move to another implementation / address deficiencies).
	return &postingsList{
		bitmap: d.bitmap.Clone(),
	}
}

func (d *postingsList) Equal(other postings.List) bool {
	if d.Len() != other.Len() {
		return false
	}

	iter := d.Iterator()
	otherIter := other.Iterator()

	for iter.Next() {
		if !otherIter.Next() {
			return false
		}
		if iter.Current() != otherIter.Current() {
			return false
		}
	}

	return true
}

type roaringIterator struct {
	iter    *roaring.Iterator
	current postings.ID
	closed  bool
}

func (it *roaringIterator) Current() postings.ID {
	return it.current
}

func (it *roaringIterator) Next() bool {
	if it.closed {
		return false
	}
	v, eof := it.iter.Next()
	if eof {
		return false
	}
	it.current = postings.ID(v)
	return true
}

func (it *roaringIterator) Err() error {
	return nil
}

func (it *roaringIterator) Close() error {
	if it.closed {
		return errIteratorClosed
	}
	it.closed = true
	return nil
}
