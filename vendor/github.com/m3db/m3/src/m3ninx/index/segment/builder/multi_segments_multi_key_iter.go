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

package builder

import (
	"bytes"

	xerrors "github.com/m3db/m3/src/x/errors"
)

type keyIterator interface {
	Next() bool
	Current() []byte
	Err() error
	Close() error
}

var _ keyIterator = &multiKeyIterator{}

type multiKeyIterator struct {
	firstNext  bool
	closeIters []keyIterator
	iters      []keyIterator
	currIters  []keyIterator
}

func newMultiKeyIterator() *multiKeyIterator {
	i := new(multiKeyIterator)
	i.reset()
	return i
}

func (i *multiKeyIterator) reset() {
	i.firstNext = true

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

func (i *multiKeyIterator) add(iter keyIterator) {
	i.closeIters = append(i.closeIters, iter)
	i.iters = append(i.iters, iter)
	i.tryAddCurr(iter)
}

func (i *multiKeyIterator) Next() bool {
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
	return true
}

func (i *multiKeyIterator) currEvaluate() {
	i.currIters = i.currIters[:0]
	for _, iter := range i.iters {
		i.tryAddCurr(iter)
	}
}

func (i *multiKeyIterator) tryAddCurr(iter keyIterator) {
	var (
		hasCurr = len(i.currIters) > 0
		cmp     int
	)
	if hasCurr {
		cmp = bytes.Compare(iter.Current(), i.Current())
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

func (i *multiKeyIterator) Current() []byte {
	return i.currIters[0].Current()
}

func (i *multiKeyIterator) CurrentIters() []keyIterator {
	return i.currIters
}

func (i *multiKeyIterator) Err() error {
	multiErr := xerrors.NewMultiError()
	for _, iter := range i.closeIters {
		multiErr = multiErr.Add(iter.Err())
	}
	return multiErr.FinalError()
}

func (i *multiKeyIterator) Close() error {
	multiErr := xerrors.NewMultiError()
	for _, iter := range i.closeIters {
		multiErr = multiErr.Add(iter.Close())
	}
	// Free resources
	i.reset()
	return multiErr.FinalError()
}
