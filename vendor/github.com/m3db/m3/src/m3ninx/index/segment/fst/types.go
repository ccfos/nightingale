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
	"fmt"
	"io"

	"github.com/m3db/m3/src/m3ninx/index"
	sgmt "github.com/m3db/m3/src/m3ninx/index/segment"
	"github.com/m3db/m3/src/x/context"
)

const (
	magicNumber = 0x6D33D0C5
)

// Version controls internal behaviour of the fst package.
type Version struct {
	Major int
	Minor int
}

var (
	// CurrentVersion describes the default current Version.
	CurrentVersion Version = Version{Major: 1, Minor: 1}

	// SupportedVersions lists all supported versions of the FST package.
	SupportedVersions = []Version{
		// 1.1 Adds support for field level metadata in a proto object,
		// and an additional postings list per Field referencing all
		// documents which have that Field.
		Version{Major: 1, Minor: 1},
		// 1.0 is the initial release.
		Version{Major: 1, Minor: 0},
	}
)

// Segment represents a FST segment.
type Segment interface {
	sgmt.ImmutableSegment
	index.Readable

	// SegmentData returns the segment data used to create the segment.
	// Note: Must close context when done with the data
	// so that can resources can be free'd safely.
	SegmentData(ctx context.Context) (SegmentData, error)
}

// Writer writes out a FST segment from the provided elements.
type Writer interface {
	// Reset sets the Writer to persist the provide segment.
	// NB(prateek): if provided segment is a mutable segment it must be sealed.
	Reset(s sgmt.Builder) error

	// MajorVersion is the major version for the writer.
	MajorVersion() int

	// MinorVersion is the minor version for the writer.
	MinorVersion() int

	// Metadata returns metadata about the writer.
	Metadata() []byte

	// WriteDocumentsData writes out the documents data to the provided writer.
	WriteDocumentsData(w io.Writer) error

	// WriteDocumentsIndex writes out the documents index to the provided writer.
	// NB(prateek): this must be called after WriteDocumentsData().
	WriteDocumentsIndex(w io.Writer) error

	// WritePostingsOffsets writes out the postings offset file to the provided
	// writer.
	WritePostingsOffsets(w io.Writer) error

	// WriteFSTTerms writes out the FSTTerms file using the provided writer.
	// NB(prateek): this must be called after WritePostingsOffsets().
	WriteFSTTerms(w io.Writer) error

	// WriteFSTFields writes out the FSTFields file using the provided writer.
	// NB(prateek): this must be called after WriteFSTTerm().
	WriteFSTFields(w io.Writer) error
}

// Supported returns an error indicating if the version is supported.
func (v Version) Supported() error {
	for _, o := range SupportedVersions {
		if v.Major == o.Major && v.Minor == o.Minor {
			return nil
		}
	}
	return fmt.Errorf("unsupported version: %+v, supported versions: %+v", v, SupportedVersions)
}

func (v Version) supportsFieldPostingsList() bool {
	return v.Major == 1 && v.Minor >= 1
}
