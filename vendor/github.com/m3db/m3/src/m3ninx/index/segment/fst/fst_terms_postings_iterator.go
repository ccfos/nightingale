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

package fst

import (
	sgmt "github.com/m3db/m3/src/m3ninx/index/segment"
	"github.com/m3db/m3/src/m3ninx/postings"
	postingsroaring "github.com/m3db/m3/src/m3ninx/postings/roaring"
	"github.com/m3dbx/pilosa/roaring"
)

// postingsIterRoaringPoolingConfig uses a configuration that avoids allocating
// any containers in the roaring bitmap, since these roaring bitmaps are backed
// by mmaps and don't have any native containers themselves.
var postingsIterRoaringPoolingConfig = roaring.ContainerPoolingConfiguration{
	MaxArraySize:                    0,
	MaxRunsSize:                     0,
	AllocateBitmap:                  false,
	MaxCapacity:                     0,
	MaxKeysAndContainersSliceLength: 128 * 10,
}

type fstTermsPostingsIter struct {
	bitmap   *roaring.Bitmap
	postings postings.List

	seg       *fsSegment
	termsIter *fstTermsIter
	currTerm  []byte
	err       error
}

func newFSTTermsPostingsIter() *fstTermsPostingsIter {
	bitmap := roaring.NewBitmapWithPooling(postingsIterRoaringPoolingConfig)
	i := &fstTermsPostingsIter{
		bitmap:   bitmap,
		postings: postingsroaring.NewPostingsListFromBitmap(bitmap),
	}
	i.clear()
	return i
}

var _ sgmt.TermsIterator = &fstTermsPostingsIter{}

func (f *fstTermsPostingsIter) clear() {
	f.bitmap.Reset()
	f.seg = nil
	f.termsIter = nil
	f.currTerm = nil
	f.err = nil
}

func (f *fstTermsPostingsIter) reset(
	seg *fsSegment,
	termsIter *fstTermsIter,
) {
	f.clear()

	f.seg = seg
	f.termsIter = termsIter
}

func (f *fstTermsPostingsIter) Next() bool {
	if f.err != nil {
		return false
	}

	next := f.termsIter.Next()
	if !next {
		return false
	}

	f.currTerm = f.termsIter.Current()
	currOffset := f.termsIter.CurrentOffset()

	f.seg.RLock()
	f.err = f.seg.unmarshalPostingsListBitmapNotClosedMaybeFinalizedWithLock(f.bitmap,
		currOffset)
	f.seg.RUnlock()

	return f.err == nil
}

func (f *fstTermsPostingsIter) Current() ([]byte, postings.List) {
	return f.currTerm, f.postings
}

func (f *fstTermsPostingsIter) Err() error {
	return f.err
}

func (f *fstTermsPostingsIter) Close() error {
	var err error
	if f.termsIter != nil {
		err = f.termsIter.Close()
	}
	f.clear()
	return err
}
