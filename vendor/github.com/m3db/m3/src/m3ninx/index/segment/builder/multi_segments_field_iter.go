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
	"github.com/m3db/m3/src/m3ninx/index/segment"
	xerrors "github.com/m3db/m3/src/x/errors"
)

// Ensure for our use case that the multi key iterator we return
// matches the signature for the fields iterator.
var _ segment.FieldsIterator = &multiKeyIterator{}

func newFieldIterFromSegments(
	segments []segmentMetadata,
) (segment.FieldsIterator, error) {
	multiIter := newMultiKeyIterator()
	for _, seg := range segments {
		iter, err := seg.segment.FieldsIterable().Fields()
		if err != nil {
			return nil, err
		}
		if !iter.Next() {
			// Don't consume this iterator if no results.
			if err := xerrors.FirstError(iter.Err(), iter.Close()); err != nil {
				return nil, err
			}
			continue
		}

		multiIter.add(iter)
	}

	return multiIter, nil
}
