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

package fst

import (
	"io"

	"github.com/m3db/m3/src/m3ninx/index"
	"github.com/m3db/m3/src/m3ninx/index/segment/fst/encoding/docs"
)

// DocumentsWriter writes out documents data given a doc iterator.
type DocumentsWriter struct {
	iter           index.IDDocIterator
	sizeHint       int
	docDataWriter  *docs.DataWriter
	docIndexWriter *docs.IndexWriter
	docOffsets     []docOffset
}

// NewDocumentsWriter creates a new documents writer.
func NewDocumentsWriter() (*DocumentsWriter, error) {
	return &DocumentsWriter{
		docDataWriter:  docs.NewDataWriter(nil),
		docIndexWriter: docs.NewIndexWriter(nil),
		docOffsets:     make([]docOffset, 0, defaultInitialDocOffsetsSize),
	}, nil
}

// DocumentsWriterOptions is a set of options to pass to the documents writer.
type DocumentsWriterOptions struct {
	// Iter is the ID and document iterator, required.
	Iter index.IDDocIterator
	// SizeHint is the size hint, optional.
	SizeHint int
}

// Reset the documents writer for writing out.
func (w *DocumentsWriter) Reset(opts DocumentsWriterOptions) {
	w.iter = opts.Iter
	w.sizeHint = opts.SizeHint
	w.docDataWriter.Reset(nil)
	w.docIndexWriter.Reset(nil)
	w.docOffsets = w.docOffsets[:0]
}

// WriteDocumentsData writes out the documents data.
func (w *DocumentsWriter) WriteDocumentsData(iow io.Writer) error {
	w.docDataWriter.Reset(iow)

	var currOffset uint64
	if cap(w.docOffsets) < w.sizeHint {
		w.docOffsets = make([]docOffset, 0, w.sizeHint)
	}
	for w.iter.Next() {
		id, doc := w.iter.PostingsID(), w.iter.Current()
		n, err := w.docDataWriter.Write(doc)
		if err != nil {
			return err
		}
		w.docOffsets = append(w.docOffsets, docOffset{ID: id, offset: currOffset})
		currOffset += uint64(n)
	}

	return nil
}

// WriteDocumentsIndex writes out the documents index data.
func (w *DocumentsWriter) WriteDocumentsIndex(iow io.Writer) error {
	w.docIndexWriter.Reset(iow)
	for _, do := range w.docOffsets {
		if err := w.docIndexWriter.Write(do.ID, do.offset); err != nil {
			return err
		}
	}
	return nil
}
