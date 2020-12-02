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

package mem

import (
	"io"

	"github.com/m3db/m3/src/m3ninx/index"
	sgmt "github.com/m3db/m3/src/m3ninx/index/segment"
	"github.com/m3db/m3/src/m3ninx/x"
)

// Merge merges the segments `srcs` into `target`.
func Merge(target sgmt.MutableSegment, srcs ...sgmt.MutableSegment) error {
	safeClosers := []io.Closer{}
	defer func() {
		for _, c := range safeClosers {
			c.Close()
		}
	}()

	// for each src
	for _, src := range srcs {
		// get reader for `src`
		reader, err := src.Reader()
		if err != nil {
			return err
		}

		// ensure readers are all closed.
		readerCloser := x.NewSafeCloser(reader)
		safeClosers = append(safeClosers, readerCloser)

		// retrieve all docs known to the reader
		dIter, err := reader.AllDocs()
		if err != nil {
			return err
		}

		// iterate over all known docs
		for dIter.Next() {
			d := dIter.Current()
			_, err := target.Insert(d)
			if err == nil || err == index.ErrDuplicateID {
				continue
			}
			return err
		}

		// ensure no errors while iterating
		if err := dIter.Err(); err != nil {
			return err
		}

		// ensure no errors while closing reader
		if err := readerCloser.Close(); err != nil {
			return err
		}
	}

	// all good
	return nil
}
