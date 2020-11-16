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
	"fmt"
	"io"
	"regexp"

	"github.com/m3db/m3/src/m3ninx/index/segment"
	"github.com/m3db/m3/src/x/mmap"
)

var (
	// TypeRegex allows what can be used for a IndexSegmentType or
	// IndexSegmentFileType, explicitly cannot use "-" as that can be
	// then used as a separator elsewhere, also it ensures callers must
	// use lower cased strings.
	TypeRegex = regexp.MustCompile("^[a-z_]+$")
)

// IndexFileSetWriter is an index file set writer, it can write out many
// segments.
type IndexFileSetWriter interface {
	// WriteSegmentFileSet writes a index segment file set.
	WriteSegmentFileSet(segmentFileSet IndexSegmentFileSetWriter) error
}

// IndexSegmentFileSetWriter is an index segment file set writer.
type IndexSegmentFileSetWriter interface {
	SegmentType() IndexSegmentType
	MajorVersion() int
	MinorVersion() int
	SegmentMetadata() []byte
	Files() []IndexSegmentFileType
	WriteFile(fileType IndexSegmentFileType, writer io.Writer) error
}

// MutableSegmentFileSetWriter is a new IndexSegmentFileSetWriter for writing
// out Mutable Segments.
type MutableSegmentFileSetWriter interface {
	IndexSegmentFileSetWriter

	// Reset resets the writer to write the provided mutable segment.
	Reset(segment.Builder) error
}

// IndexFileSetReader is an index file set reader, it can read many segments.
type IndexFileSetReader interface {
	// SegmentFileSets returns the number of segment file sets.
	SegmentFileSets() int

	// ReadSegmentFileSet returns the next segment file set or an error.
	// It will return io.EOF error when no more file sets remain.
	// The IndexSegmentFileSet will only be valid before it's closed,
	// after that calls to Read or Bytes on it will have unexpected results.
	ReadSegmentFileSet() (IndexSegmentFileSet, error)

	IndexVolumeType() IndexVolumeType
}

// IndexSegmentFileSet is an index segment file set.
type IndexSegmentFileSet interface {
	SegmentType() IndexSegmentType
	MajorVersion() int
	MinorVersion() int
	SegmentMetadata() []byte
	Files() []IndexSegmentFile
}

// IndexSegmentFile is a file in an index segment file set.
type IndexSegmentFile interface {
	io.Reader
	io.Closer

	// SegmentFileType returns the segment file type.
	SegmentFileType() IndexSegmentFileType

	// Mmap will be valid until the segment file is closed.
	Mmap() (mmap.Descriptor, error)
}

// IndexVolumeType is the type of an index volume.
type IndexVolumeType string

const (
	// DefaultIndexVolumeType is a default IndexVolumeType.
	// This is the type if not otherwise specified.
	DefaultIndexVolumeType IndexVolumeType = "default"
)

// IndexSegmentType is the type of an index file set.
type IndexSegmentType string

const (
	// FSTIndexSegmentType is a FST IndexSegmentType.
	FSTIndexSegmentType IndexSegmentType = "fst"
)

// IndexSegmentFileType is the type of a file in an index file set.
type IndexSegmentFileType string

const (
	// DocumentDataIndexSegmentFileType is a document data segment file.
	DocumentDataIndexSegmentFileType IndexSegmentFileType = "docdata"

	// DocumentIndexIndexSegmentFileType is a document index segment file.
	DocumentIndexIndexSegmentFileType IndexSegmentFileType = "docidx"

	// PostingsIndexSegmentFileType is a postings List data index segment file.
	PostingsIndexSegmentFileType IndexSegmentFileType = "postingsdata"

	// FSTFieldsIndexSegmentFileType is a FST Fields index segment file.
	FSTFieldsIndexSegmentFileType IndexSegmentFileType = "fstfields"

	// FSTTermsIndexSegmentFileType is a FST Terms index segment file.
	FSTTermsIndexSegmentFileType IndexSegmentFileType = "fstterms"
)

var (
	indexSegmentTypes = []IndexSegmentType{
		FSTIndexSegmentType,
	}

	indexSegmentFileTypes = []IndexSegmentFileType{
		DocumentDataIndexSegmentFileType,
		DocumentIndexIndexSegmentFileType,
		PostingsIndexSegmentFileType,
		FSTFieldsIndexSegmentFileType,
		FSTTermsIndexSegmentFileType,
	}
)

func init() {
	for _, f := range indexSegmentTypes {
		if err := f.Validate(); err != nil {
			panic(err)
		}
	}
	for _, f := range indexSegmentFileTypes {
		if err := f.Validate(); err != nil {
			panic(err)
		}
	}
}

// Validate validates whether the string value is a valid segment type
// and contains only lowercase a-z and underscore characters.
func (t IndexSegmentType) Validate() error {
	s := string(t)
	if t == "" || !TypeRegex.MatchString(s) {
		return fmt.Errorf("invalid segment type must match pattern=%s",
			TypeRegex.String())
	}
	return nil
}

// Validate validates whether the string value is a valid segment file type
// and contains only lowercase a-z and underscore characters.
func (t IndexSegmentFileType) Validate() error {
	s := string(t)
	if t == "" || !TypeRegex.MatchString(s) {
		return fmt.Errorf("invalid segment file type must match pattern=%s",
			TypeRegex.String())
	}
	return nil
}
