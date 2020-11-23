// Copyright (c) 2020 Uber Technologies, Inc.
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
	"bytes"

	"github.com/m3db/m3/src/m3ninx/index"
	"github.com/m3db/m3/src/m3ninx/index/segment"
	"github.com/m3db/m3/src/m3ninx/postings"
	"github.com/m3db/m3/src/m3ninx/postings/roaring"
	xerrors "github.com/m3db/m3/src/x/errors"
	bitmap "github.com/m3dbx/pilosa/roaring"
)

var _ segment.FieldsPostingsListIterator = &multiKeyPostingsListIterator{}

type multiKeyPostingsListIterator struct {
	err                   error
	firstNext             bool
	closeIters            []keyIterator
	iters                 []keyIterator
	currIters             []keyIterator
	currReaders           []index.Reader
	currFieldPostingsList postings.MutableList
	bitmapIter            *bitmap.Iterator
}

func newMultiKeyPostingsListIterator() *multiKeyPostingsListIterator {
	b := bitmap.NewBitmapWithDefaultPooling(defaultBitmapContainerPooling)
	i := &multiKeyPostingsListIterator{
		currFieldPostingsList: roaring.NewPostingsListFromBitmap(b),
		bitmapIter:            &bitmap.Iterator{},
	}
	i.reset()
	return i
}

func (i *multiKeyPostingsListIterator) reset() {
	i.firstNext = true
	i.currFieldPostingsList.Reset()

	for j := range i.closeIters {
		i.closeIters[j] = nil
	}
	i.closeIters = i.closeIters[:0]

	for j := range i.iters {
		i.iters[j] = nil
	}
	i.iters = i.iters[:0]

	for j := range i.currIters {
		i.currIters[j] = nil
	}
	i.currIters = i.currIters[:0]
}

func (i *multiKeyPostingsListIterator) add(iter keyIterator) {
	i.closeIters = append(i.closeIters, iter)
	i.iters = append(i.iters, iter)
	i.tryAddCurr(iter)
}

func (i *multiKeyPostingsListIterator) Next() bool {
	if i.err != nil {
		return false
	}
	if len(i.iters) == 0 {
		return false
	}

	if i.firstNext {
		i.firstNext = false
		return true
	}

	for _, currIter := range i.currIters {
		currNext := currIter.Next()
		if currNext {
			// Next has a value, forward other matching too
			continue
		}

		// Remove iter
		n := len(i.iters)
		idx := -1
		for j, iter := range i.iters {
			if iter == currIter {
				idx = j
				break
			}
		}
		i.iters[idx] = i.iters[n-1]
		i.iters[n-1] = nil
		i.iters = i.iters[:n-1]
	}
	if len(i.iters) == 0 {
		return false
	}

	// Re-evaluate current value
	i.currEvaluate()

	// NB(bodu): Build the postings list for this field if the field has changed.
	defer func() {
		for idx, reader := range i.currReaders {
			if err := reader.Close(); err != nil {
				i.err = err
			}
			i.currReaders[idx] = nil
		}
		i.currReaders = i.currReaders[:0]
	}()

	i.currFieldPostingsList.Reset()
	currField := i.currIters[0].Current()

	for _, iter := range i.currIters {
		fieldsKeyIter := iter.(*fieldsKeyIter)
		reader, err := fieldsKeyIter.segment.segment.Reader()
		if err != nil {
			i.err = err
			return false
		}
		i.currReaders = append(i.currReaders, reader)

		pl, err := reader.MatchField(currField)
		if err != nil {
			i.err = err
			return false
		}

		if fieldsKeyIter.segment.offset == 0 {
			// No offset, which means is first segment we are combining from
			// so can just direct union
			i.currFieldPostingsList.Union(pl)
			continue
		}

		// We have to taken into account the offset and duplicates
		var (
			iter           = i.bitmapIter
			duplicates     = fieldsKeyIter.segment.duplicatesAsc
			negativeOffset postings.ID
		)
		bitmap, ok := roaring.BitmapFromPostingsList(pl)
		if !ok {
			i.err = errPostingsListNotRoaring
			return false
		}

		iter.Reset(bitmap)
		for v, eof := iter.Next(); !eof; v, eof = iter.Next() {
			curr := postings.ID(v)
			for len(duplicates) > 0 && curr > duplicates[0] {
				duplicates = duplicates[1:]
				negativeOffset++
			}
			if len(duplicates) > 0 && curr == duplicates[0] {
				duplicates = duplicates[1:]
				negativeOffset++
				// Also skip this value, as itself is a duplicate
				continue
			}
			value := curr + fieldsKeyIter.segment.offset - negativeOffset
			if err := i.currFieldPostingsList.Insert(value); err != nil {
				i.err = err
				return false
			}
		}
	}
	return true
}

func (i *multiKeyPostingsListIterator) currEvaluate() {
	i.currIters = i.currIters[:0]
	for _, iter := range i.iters {
		i.tryAddCurr(iter)
	}
}

func (i *multiKeyPostingsListIterator) tryAddCurr(iter keyIterator) {
	var (
		hasCurr = len(i.currIters) > 0
		cmp     int
	)
	if hasCurr {
		curr, _ := i.Current()
		cmp = bytes.Compare(iter.Current(), curr)
	}
	if !hasCurr || cmp < 0 {
		// Set the current lowest key value
		i.currIters = i.currIters[:0]
		i.currIters = append(i.currIters, iter)
	} else if hasCurr && cmp == 0 {
		// Set a matching duplicate curr iter
		i.currIters = append(i.currIters, iter)
	}
}

func (i *multiKeyPostingsListIterator) Current() ([]byte, postings.List) {
	return i.currIters[0].Current(), i.currFieldPostingsList
}

func (i *multiKeyPostingsListIterator) CurrentIters() []keyIterator {
	return i.currIters
}

func (i *multiKeyPostingsListIterator) Err() error {
	multiErr := xerrors.NewMultiError()
	for _, iter := range i.closeIters {
		multiErr = multiErr.Add(iter.Err())
	}
	if i.err != nil {
		multiErr = multiErr.Add(i.err)
	}
	return multiErr.FinalError()
}

func (i *multiKeyPostingsListIterator) Close() error {
	multiErr := xerrors.NewMultiError()
	for _, iter := range i.closeIters {
		multiErr = multiErr.Add(iter.Close())
	}
	// Free resources
	i.reset()
	return multiErr.FinalError()
}
