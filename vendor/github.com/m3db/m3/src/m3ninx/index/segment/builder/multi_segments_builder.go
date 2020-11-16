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
	"io"
	"sort"

	"github.com/m3db/m3/src/m3ninx/doc"
	"github.com/m3db/m3/src/m3ninx/index"
	"github.com/m3db/m3/src/m3ninx/index/segment"
	"github.com/m3db/m3/src/m3ninx/postings"
	xerrors "github.com/m3db/m3/src/x/errors"
)

type builderFromSegments struct {
	docs           []doc.Document
	idSet          *IDsMap
	segments       []segmentMetadata
	termsIter      *termsIterFromSegments
	segmentsOffset postings.ID
}

type segmentMetadata struct {
	segment segment.Segment
	offset  postings.ID
	// duplicatesAsc is a lookup of document IDs are duplicates
	// in this segment, that is documents that are already
	// contained by other segments and hence should not be
	// returned when looking up documents.
	duplicatesAsc []postings.ID
}

// NewBuilderFromSegments returns a new builder from segments.
func NewBuilderFromSegments(opts Options) segment.SegmentsBuilder {
	return &builderFromSegments{
		idSet: NewIDsMap(IDsMapOptions{
			InitialSize: opts.InitialCapacity(),
		}),
		termsIter: newTermsIterFromSegments(),
	}
}

func (b *builderFromSegments) Reset() {
	// Reset the documents slice
	var emptyDoc doc.Document
	for i := range b.docs {
		b.docs[i] = emptyDoc
	}
	b.docs = b.docs[:0]

	// Reset all entries in ID set
	b.idSet.Reset()

	// Reset the segments metadata
	b.segmentsOffset = 0
	var emptySegment segmentMetadata
	for i := range b.segments {
		b.segments[i] = emptySegment
	}
	b.segments = b.segments[:0]

	b.termsIter.clear()
}

func (b *builderFromSegments) AddSegments(segments []segment.Segment) error {
	// numMaxDocs can sometimes be larger than the actual number of documents
	// since some are duplicates
	numMaxDocs := 0
	for _, segment := range segments {
		numMaxDocs += int(segment.Size())
	}

	// Ensure we don't have to constantly reallocate docs slice
	totalMaxSize := len(b.docs) + numMaxDocs
	if cap(b.docs) < totalMaxSize {
		b.docs = make([]doc.Document, 0, totalMaxSize)
	}

	// First build metadata and docs slice
	for _, segment := range segments {
		iter, closer, err := allDocsIter(segment)
		if err != nil {
			return err
		}

		var (
			added      int
			duplicates []postings.ID
		)
		for iter.Next() {
			d := iter.Current()
			if b.idSet.Contains(d.ID) {
				duplicates = append(duplicates, iter.PostingsID())
				continue
			}
			b.idSet.SetUnsafe(d.ID, struct{}{}, IDsMapSetUnsafeOptions{
				NoCopyKey:     true,
				NoFinalizeKey: true,
			})
			b.docs = append(b.docs, d)
			added++
		}

		err = xerrors.FirstError(iter.Err(), iter.Close(), closer.Close())
		if err != nil {
			return err
		}

		// Sort duplicates in ascending order
		sort.Slice(duplicates, func(i, j int) bool {
			return duplicates[i] < duplicates[j]
		})

		b.segments = append(b.segments, segmentMetadata{
			segment:       segment,
			offset:        b.segmentsOffset,
			duplicatesAsc: duplicates,
		})
		b.segmentsOffset += postings.ID(added)
	}

	// Make sure the terms iter has all the segments to combine data from
	b.termsIter.reset(b.segments)

	return nil
}

func (b *builderFromSegments) Docs() []doc.Document {
	return b.docs
}

func (b *builderFromSegments) AllDocs() (index.IDDocIterator, error) {
	rangeIter := postings.NewRangeIterator(0, postings.ID(len(b.docs)))
	return index.NewIDDocIterator(b, rangeIter), nil
}

func (b *builderFromSegments) Doc(id postings.ID) (doc.Document, error) {
	idx := int(id)
	if idx < 0 || idx >= len(b.docs) {
		return doc.Document{}, errDocNotFound
	}

	return b.docs[idx], nil
}

func (b *builderFromSegments) FieldsIterable() segment.FieldsIterable {
	return b
}

func (b *builderFromSegments) TermsIterable() segment.TermsIterable {
	return b
}

func (b *builderFromSegments) Fields() (segment.FieldsIterator, error) {
	return newFieldIterFromSegments(b.segments)
}

func (b *builderFromSegments) FieldsPostingsList() (segment.FieldsPostingsListIterator, error) {
	return newFieldPostingsListIterFromSegments(b.segments)
}

func (b *builderFromSegments) Terms(field []byte) (segment.TermsIterator, error) {
	if err := b.termsIter.setField(field); err != nil {
		return nil, err
	}
	return b.termsIter, nil
}

func allDocsIter(seg segment.Segment) (index.IDDocIterator, io.Closer, error) {
	reader, err := seg.Reader()
	if err != nil {
		return nil, nil, err
	}

	iter, err := reader.AllDocs()
	if err != nil {
		return nil, nil, err
	}

	return iter, reader, nil
}
