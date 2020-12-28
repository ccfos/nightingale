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

package persist

import (
	"errors"
	"fmt"
	"io"

	"github.com/m3db/m3/src/m3ninx/index/segment"
	"github.com/m3db/m3/src/m3ninx/index/segment/fst"
	"github.com/m3db/m3/src/m3ninx/x"
)

var (
	errDocsDataFileNotWritten = errors.New("docs data file must be written before index data")
)

// NewMutableSegmentFileSetWriter returns a new IndexSegmentFileSetWriter for writing
// out the provided Mutable Segment.
func NewMutableSegmentFileSetWriter(
	fstOpts fst.WriterOptions,
) (MutableSegmentFileSetWriter, error) {
	w, err := fst.NewWriter(fstOpts)
	if err != nil {
		return nil, err
	}
	return newMutableSegmentFileSetWriter(w)
}

func newMutableSegmentFileSetWriter(
	fsWriter fst.Writer,
) (MutableSegmentFileSetWriter, error) {
	return &writer{
		fsWriter: fsWriter,
	}, nil
}

type writer struct {
	fsWriter fst.Writer
}

func (w *writer) Reset(builder segment.Builder) error {
	return w.fsWriter.Reset(builder)
}

func (w *writer) SegmentType() IndexSegmentType {
	return FSTIndexSegmentType
}

func (w *writer) MajorVersion() int {
	return w.fsWriter.MajorVersion()
}

func (w *writer) MinorVersion() int {
	return w.fsWriter.MinorVersion()
}

func (w *writer) SegmentMetadata() []byte {
	return w.fsWriter.Metadata()
}

func (w *writer) Files() []IndexSegmentFileType {
	// NB(prateek): order is important here. It is the order of files written out,
	// and needs to be maintained as it is below.
	return []IndexSegmentFileType{
		DocumentDataIndexSegmentFileType,
		DocumentIndexIndexSegmentFileType,
		PostingsIndexSegmentFileType,
		FSTTermsIndexSegmentFileType,
		FSTFieldsIndexSegmentFileType,
	}
}

func (w *writer) WriteFile(fileType IndexSegmentFileType, iow io.Writer) error {
	switch fileType {
	case DocumentDataIndexSegmentFileType:
		return w.fsWriter.WriteDocumentsData(iow)
	case DocumentIndexIndexSegmentFileType:
		return w.fsWriter.WriteDocumentsIndex(iow)
	case PostingsIndexSegmentFileType:
		return w.fsWriter.WritePostingsOffsets(iow)
	case FSTFieldsIndexSegmentFileType:
		return w.fsWriter.WriteFSTFields(iow)
	case FSTTermsIndexSegmentFileType:
		return w.fsWriter.WriteFSTTerms(iow)
	}
	return fmt.Errorf("unknown fileType: %s provided", fileType)
}

// NewFSTSegmentDataFileSetWriter creates a new file set writer for
// fst segment data.
func NewFSTSegmentDataFileSetWriter(
	data fst.SegmentData,
) (IndexSegmentFileSetWriter, error) {
	if err := data.Validate(); err != nil {
		return nil, err
	}

	docsWriter, err := fst.NewDocumentsWriter()
	if err != nil {
		return nil, err
	}

	return &fstSegmentDataWriter{
		data:       data,
		docsWriter: docsWriter,
	}, nil
}

type fstSegmentDataWriter struct {
	data                fst.SegmentData
	docsWriter          *fst.DocumentsWriter
	docsDataFileWritten bool
}

func (w *fstSegmentDataWriter) SegmentType() IndexSegmentType {
	return FSTIndexSegmentType
}

func (w *fstSegmentDataWriter) MajorVersion() int {
	return w.data.Version.Major
}

func (w *fstSegmentDataWriter) MinorVersion() int {
	return w.data.Version.Minor
}

func (w *fstSegmentDataWriter) SegmentMetadata() []byte {
	return w.data.Metadata
}

func (w *fstSegmentDataWriter) Files() []IndexSegmentFileType {
	return []IndexSegmentFileType{
		DocumentDataIndexSegmentFileType,
		DocumentIndexIndexSegmentFileType,
		PostingsIndexSegmentFileType,
		FSTTermsIndexSegmentFileType,
		FSTFieldsIndexSegmentFileType,
	}
}

func (w *fstSegmentDataWriter) WriteFile(fileType IndexSegmentFileType, iow io.Writer) error {
	switch fileType {
	case DocumentDataIndexSegmentFileType:
		if err := w.writeDocsData(iow); err != nil {
			return err
		}
		w.docsDataFileWritten = true
		return nil
	case DocumentIndexIndexSegmentFileType:
		if !w.docsDataFileWritten {
			return errDocsDataFileNotWritten
		}
		return w.writeDocsIndex(iow)
	case PostingsIndexSegmentFileType:
		_, err := iow.Write(w.data.PostingsData.Bytes)
		return err
	case FSTFieldsIndexSegmentFileType:
		_, err := iow.Write(w.data.FSTFieldsData.Bytes)
		return err
	case FSTTermsIndexSegmentFileType:
		_, err := iow.Write(w.data.FSTTermsData.Bytes)
		return err
	}
	return fmt.Errorf("unknown fileType: %s provided", fileType)
}

func (w *fstSegmentDataWriter) writeDocsData(iow io.Writer) error {
	if r := w.data.DocsReader; r != nil {
		iter := r.Iter()
		closer := x.NewSafeCloser(iter)
		defer closer.Close()
		w.docsWriter.Reset(fst.DocumentsWriterOptions{
			Iter:     iter,
			SizeHint: r.Len(),
		})
		return w.docsWriter.WriteDocumentsData(iow)
	}

	_, err := iow.Write(w.data.DocsData.Bytes)
	return err
}

func (w *fstSegmentDataWriter) writeDocsIndex(iow io.Writer) error {
	if r := w.data.DocsReader; r != nil {
		return w.docsWriter.WriteDocumentsIndex(iow)
	}

	_, err := iow.Write(w.data.DocsData.Bytes)
	return err
}
