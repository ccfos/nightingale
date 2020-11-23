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
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/m3db/m3/src/m3ninx/doc"
	"github.com/m3db/m3/src/m3ninx/generated/proto/fswriter"
	"github.com/m3db/m3/src/m3ninx/index"
	"github.com/m3db/m3/src/m3ninx/index/segment"
	sgmt "github.com/m3db/m3/src/m3ninx/index/segment"
	"github.com/m3db/m3/src/m3ninx/index/segment/fst/encoding"
	"github.com/m3db/m3/src/m3ninx/index/segment/fst/encoding/docs"
	"github.com/m3db/m3/src/m3ninx/postings"
	"github.com/m3db/m3/src/m3ninx/postings/pilosa"
	"github.com/m3db/m3/src/m3ninx/postings/roaring"
	"github.com/m3db/m3/src/m3ninx/x"
	"github.com/m3db/m3/src/x/context"
	xerrors "github.com/m3db/m3/src/x/errors"
	"github.com/m3db/m3/src/x/mmap"

	pilosaroaring "github.com/m3dbx/pilosa/roaring"
	"github.com/m3dbx/vellum"
)

var (
	errReaderClosed            = errors.New("segment is closed")
	errReaderFinalized         = errors.New("segment is finalized")
	errReaderNilRegexp         = errors.New("nil regexp provided")
	errUnsupportedMajorVersion = errors.New("unsupported major version")
	errDocumentsDataUnset      = errors.New("documents data bytes are not set")
	errDocumentsIdxUnset       = errors.New("documents index bytes are not set")
	errPostingsDataUnset       = errors.New("postings data bytes are not set")
	errFSTTermsDataUnset       = errors.New("fst terms data bytes are not set")
	errFSTFieldsDataUnset      = errors.New("fst fields data bytes are not set")
)

// SegmentData represent the collection of required parameters to construct a Segment.
type SegmentData struct {
	Version  Version
	Metadata []byte

	DocsData      mmap.Descriptor
	DocsIdxData   mmap.Descriptor
	PostingsData  mmap.Descriptor
	FSTTermsData  mmap.Descriptor
	FSTFieldsData mmap.Descriptor

	// DocsReader is an alternative to specifying
	// the docs data and docs idx data if the documents
	// already reside in memory and we want to use the
	// in memory references instead.
	DocsReader docs.Reader

	Closer io.Closer
}

// Validate validates the provided segment data, returning an error if it's not.
func (sd SegmentData) Validate() error {
	if err := sd.Version.Supported(); err != nil {
		return err
	}

	if sd.PostingsData.Bytes == nil {
		return errPostingsDataUnset
	}

	if sd.FSTTermsData.Bytes == nil {
		return errFSTTermsDataUnset
	}

	if sd.FSTFieldsData.Bytes == nil {
		return errFSTFieldsDataUnset
	}

	if sd.DocsReader == nil {
		if sd.DocsData.Bytes == nil {
			return errDocumentsDataUnset
		}

		if sd.DocsIdxData.Bytes == nil {
			return errDocumentsIdxUnset
		}
	}

	return nil
}

// NewSegment returns a new Segment backed by the provided options.
// NB(prateek): this method only assumes ownership of the data if it returns a nil error,
// otherwise, the user is expected to handle the lifecycle of the input.
func NewSegment(data SegmentData, opts Options) (Segment, error) {
	if err := data.Validate(); err != nil {
		return nil, err
	}

	metadata := fswriter.Metadata{}
	if err := metadata.Unmarshal(data.Metadata); err != nil {
		return nil, err
	}

	if metadata.PostingsFormat != fswriter.PostingsFormat_PILOSAV1_POSTINGS_FORMAT {
		return nil, fmt.Errorf("unsupported postings format: %v", metadata.PostingsFormat.String())
	}

	fieldsFST, err := vellum.Load(data.FSTFieldsData.Bytes)
	if err != nil {
		return nil, fmt.Errorf("unable to load fields fst: %v", err)
	}

	var (
		docsThirdPartyReader = data.DocsReader
		docsDataReader       *docs.DataReader
		docsIndexReader      *docs.IndexReader
	)
	if docsThirdPartyReader == nil {
		docsDataReader = docs.NewDataReader(data.DocsData.Bytes)
		docsIndexReader, err = docs.NewIndexReader(data.DocsIdxData.Bytes)
		if err != nil {
			return nil, fmt.Errorf("unable to load documents index: %v", err)
		}
	}

	s := &fsSegment{
		fieldsFST:            fieldsFST,
		docsDataReader:       docsDataReader,
		docsIndexReader:      docsIndexReader,
		docsThirdPartyReader: docsThirdPartyReader,

		data:    data,
		opts:    opts,
		numDocs: metadata.NumDocs,
	}

	// NB(r): The segment uses the context finalization to finalize
	// resources. Finalize is called after Close is called and all
	// the segment readers have also been closed.
	s.ctx = opts.ContextPool().Get()
	s.ctx.RegisterFinalizer(s)

	return s, nil
}

// Ensure FST segment implements ImmutableSegment so can be casted upwards
// and mmap's can be freed.
var _ segment.ImmutableSegment = (*fsSegment)(nil)

type fsSegment struct {
	sync.RWMutex
	ctx                  context.Context
	closed               bool
	finalized            bool
	fieldsFST            *vellum.FST
	docsDataReader       *docs.DataReader
	docsIndexReader      *docs.IndexReader
	docsThirdPartyReader docs.Reader
	data                 SegmentData
	opts                 Options

	numDocs int64
}

func (r *fsSegment) SegmentData(ctx context.Context) (SegmentData, error) {
	r.RLock()
	defer r.RUnlock()
	if r.closed {
		return SegmentData{}, errReaderClosed
	}

	// NB(r): Ensure that we do not release, mmaps, etc
	// until all readers have been closed.
	r.ctx.DependsOn(ctx)
	return r.data, nil
}

func (r *fsSegment) Size() int64 {
	r.RLock()
	defer r.RUnlock()
	if r.closed {
		return 0
	}
	return r.numDocs
}

func (r *fsSegment) ContainsID(docID []byte) (bool, error) {
	r.RLock()
	defer r.RUnlock()
	if r.closed {
		return false, errReaderClosed
	}

	termsFST, exists, err := r.retrieveTermsFSTWithRLock(doc.IDReservedFieldName)
	if err != nil {
		return false, err
	}

	if !exists {
		return false, fmt.Errorf("internal error while retrieving id FST: %v", err)
	}

	_, exists, err = termsFST.Get(docID)
	closeErr := termsFST.Close()
	if err != nil {
		return false, err
	}

	return exists, closeErr
}

func (r *fsSegment) ContainsField(field []byte) (bool, error) {
	r.RLock()
	defer r.RUnlock()
	if r.closed {
		return false, errReaderClosed
	}
	return r.fieldsFST.Contains(field)
}

func (r *fsSegment) Reader() (sgmt.Reader, error) {
	r.RLock()
	defer r.RUnlock()
	if r.closed {
		return nil, errReaderClosed
	}

	reader := newReader(r, r.opts)

	// NB(r): Ensure that we do not release, mmaps, etc
	// until all readers have been closed.
	r.ctx.DependsOn(reader.ctx)

	return reader, nil
}

func (r *fsSegment) Close() error {
	r.Lock()
	if r.closed {
		r.Unlock()
		return errReaderClosed
	}
	r.closed = true
	r.Unlock()
	// NB(r): Inform context we are done, once all segment readers are
	// closed the segment Finalize will be called async.
	r.ctx.Close()
	return nil
}

func (r *fsSegment) Finalize() {
	r.Lock()
	r.fieldsFST.Close()
	if r.data.Closer != nil {
		r.data.Closer.Close()
	}
	r.finalized = true
	r.Unlock()
}

func (r *fsSegment) FieldsIterable() sgmt.FieldsIterable {
	return r
}

func (r *fsSegment) Fields() (sgmt.FieldsIterator, error) {
	r.RLock()
	defer r.RUnlock()
	if r.closed {
		return nil, errReaderClosed
	}

	iter := newFSTTermsIter()
	iter.reset(fstTermsIterOpts{
		seg:         r,
		fst:         r.fieldsFST,
		finalizeFST: false,
	})
	return iter, nil
}

func (r *fsSegment) TermsIterable() sgmt.TermsIterable {
	return &termsIterable{
		r:            r,
		fieldsIter:   newFSTTermsIter(),
		postingsIter: newFSTTermsPostingsIter(),
	}
}

func (r *fsSegment) FreeMmap() error {
	multiErr := xerrors.NewMultiError()

	// NB(bodu): PostingsData, FSTTermsData and FSTFieldsData always present.
	if err := mmap.MadviseDontNeed(r.data.PostingsData); err != nil {
		multiErr = multiErr.Add(err)
	}
	if err := mmap.MadviseDontNeed(r.data.FSTTermsData); err != nil {
		multiErr = multiErr.Add(err)
	}
	if err := mmap.MadviseDontNeed(r.data.FSTFieldsData); err != nil {
		multiErr = multiErr.Add(err)
	}

	// DocsData and DocsIdxData are not always present.
	if r.data.DocsData.Bytes != nil {
		if err := mmap.MadviseDontNeed(r.data.DocsData); err != nil {
			multiErr = multiErr.Add(err)
		}
	}
	if r.data.DocsIdxData.Bytes != nil {
		if err := mmap.MadviseDontNeed(r.data.DocsIdxData); err != nil {
			multiErr = multiErr.Add(err)
		}
	}

	return multiErr.FinalError()
}

// termsIterable allows multiple term lookups to share the same roaring
// bitmap being unpacked for use when iterating over an entire segment
type termsIterable struct {
	r            *fsSegment
	fieldsIter   *fstTermsIter
	postingsIter *fstTermsPostingsIter
}

func newTermsIterable(r *fsSegment) *termsIterable {
	return &termsIterable{
		r:            r,
		fieldsIter:   newFSTTermsIter(),
		postingsIter: newFSTTermsPostingsIter(),
	}
}

func (i *termsIterable) Terms(field []byte) (sgmt.TermsIterator, error) {
	i.r.RLock()
	defer i.r.RUnlock()
	if i.r.closed {
		return nil, errReaderClosed
	}
	return i.termsNotClosedMaybeFinalizedWithRLock(field)
}

func (i *termsIterable) termsNotClosedMaybeFinalizedWithRLock(
	field []byte,
) (sgmt.TermsIterator, error) {
	// NB(r): Not closed, but could be finalized (i.e. closed segment reader)
	// calling match field after this segment is finalized.
	if i.r.finalized {
		return nil, errReaderFinalized
	}

	termsFST, exists, err := i.r.retrieveTermsFSTWithRLock(field)
	if err != nil {
		return nil, err
	}

	if !exists {
		return sgmt.EmptyTermsIterator, nil
	}

	i.fieldsIter.reset(fstTermsIterOpts{
		seg:         i.r,
		fst:         termsFST,
		finalizeFST: true,
	})
	i.postingsIter.reset(i.r, i.fieldsIter)
	return i.postingsIter, nil
}

func (r *fsSegment) UnmarshalPostingsListBitmap(b *pilosaroaring.Bitmap, offset uint64) error {
	r.RLock()
	defer r.RUnlock()
	if r.closed {
		return errReaderClosed
	}

	return r.unmarshalPostingsListBitmapNotClosedMaybeFinalizedWithLock(b, offset)
}

func (r *fsSegment) unmarshalPostingsListBitmapNotClosedMaybeFinalizedWithLock(b *pilosaroaring.Bitmap, offset uint64) error {
	if r.finalized {
		return errReaderFinalized
	}

	postingsBytes, err := r.retrieveBytesWithRLock(r.data.PostingsData.Bytes, offset)
	if err != nil {
		return fmt.Errorf("unable to retrieve postings data: %v", err)
	}

	b.Reset()
	return b.UnmarshalBinary(postingsBytes)
}

func (r *fsSegment) MatchField(field []byte) (postings.List, error) {
	r.RLock()
	defer r.RUnlock()
	if r.closed {
		return nil, errReaderClosed
	}
	return r.matchFieldNotClosedMaybeFinalizedWithRLock(field)
}

func (r *fsSegment) matchFieldNotClosedMaybeFinalizedWithRLock(
	field []byte,
) (postings.List, error) {
	// NB(r): Not closed, but could be finalized (i.e. closed segment reader)
	// calling match field after this segment is finalized.
	if r.finalized {
		return nil, errReaderFinalized
	}

	if !r.data.Version.supportsFieldPostingsList() {
		// i.e. don't have the field level postings list, so fall back to regexp
		return r.matchRegexpNotClosedMaybeFinalizedWithRLock(field, index.DotStarCompiledRegex())
	}

	termsFSTOffset, exists, err := r.fieldsFST.Get(field)
	if err != nil {
		return nil, err
	}
	if !exists {
		// i.e. we don't know anything about the term, so can early return an empty postings list
		return r.opts.PostingsListPool().Get(), nil
	}

	protoBytes, _, err := r.retrieveTermsBytesWithRLock(r.data.FSTTermsData.Bytes, termsFSTOffset)
	if err != nil {
		return nil, err
	}

	var fieldData fswriter.FieldData
	if err := fieldData.Unmarshal(protoBytes); err != nil {
		return nil, err
	}

	postingsOffset := fieldData.FieldPostingsListOffset
	return r.retrievePostingsListWithRLock(postingsOffset)
}

func (r *fsSegment) MatchTerm(field []byte, term []byte) (postings.List, error) {
	r.RLock()
	defer r.RUnlock()
	if r.closed {
		return nil, errReaderClosed
	}
	return r.matchTermNotClosedMaybeFinalizedWithRLock(field, term)
}

func (r *fsSegment) matchTermNotClosedMaybeFinalizedWithRLock(
	field, term []byte,
) (postings.List, error) {
	// NB(r): Not closed, but could be finalized (i.e. closed segment reader)
	// calling match field after this segment is finalized.
	if r.finalized {
		return nil, errReaderFinalized
	}

	termsFST, exists, err := r.retrieveTermsFSTWithRLock(field)
	if err != nil {
		return nil, err
	}

	if !exists {
		// i.e. we don't know anything about the field, so can early return an empty postings list
		return r.opts.PostingsListPool().Get(), nil
	}

	fstCloser := x.NewSafeCloser(termsFST)
	defer fstCloser.Close()

	postingsOffset, exists, err := termsFST.Get(term)
	if err != nil {
		return nil, err
	}

	if !exists {
		// i.e. we don't know anything about the term, so can early return an empty postings list
		return r.opts.PostingsListPool().Get(), nil
	}

	pl, err := r.retrievePostingsListWithRLock(postingsOffset)
	if err != nil {
		return nil, err
	}

	if err := fstCloser.Close(); err != nil {
		return nil, err
	}

	return pl, nil
}

func (r *fsSegment) MatchRegexp(
	field []byte,
	compiled index.CompiledRegex,
) (postings.List, error) {
	r.RLock()
	defer r.Unlock()
	if r.closed {
		return nil, errReaderClosed
	}
	return r.matchRegexpNotClosedMaybeFinalizedWithRLock(field, compiled)
}

func (r *fsSegment) matchRegexpNotClosedMaybeFinalizedWithRLock(
	field []byte,
	compiled index.CompiledRegex,
) (postings.List, error) {
	// NB(r): Not closed, but could be finalized (i.e. closed segment reader)
	// calling match field after this segment is finalized.
	if r.finalized {
		return nil, errReaderFinalized
	}

	re := compiled.FST
	if re == nil {
		return nil, errReaderNilRegexp
	}

	termsFST, exists, err := r.retrieveTermsFSTWithRLock(field)
	if err != nil {
		return nil, err
	}

	if !exists {
		// i.e. we don't know anything about the field, so can early return an empty postings list
		return r.opts.PostingsListPool().Get(), nil
	}

	var (
		fstCloser     = x.NewSafeCloser(termsFST)
		iter, iterErr = termsFST.Search(re, compiled.PrefixBegin, compiled.PrefixEnd)
		iterCloser    = x.NewSafeCloser(iter)
		// NB(prateek): way quicker to union the PLs together at the end, rathen than one at a time.
		pls []postings.List // TODO: pool this slice allocation
	)
	defer func() {
		iterCloser.Close()
		fstCloser.Close()
	}()

	for {
		if iterErr == vellum.ErrIteratorDone {
			break
		}

		if iterErr != nil {
			return nil, iterErr
		}

		_, postingsOffset := iter.Current()
		nextPl, err := r.retrievePostingsListWithRLock(postingsOffset)
		if err != nil {
			return nil, err
		}
		pls = append(pls, nextPl)
		iterErr = iter.Next()
	}

	pl, err := roaring.Union(pls)
	if err != nil {
		return nil, err
	}

	if err := iterCloser.Close(); err != nil {
		return nil, err
	}

	if err := fstCloser.Close(); err != nil {
		return nil, err
	}

	return pl, nil
}

func (r *fsSegment) MatchAll() (postings.MutableList, error) {
	r.RLock()
	defer r.RUnlock()
	if r.closed {
		return nil, errReaderClosed
	}
	return r.matchAllNotClosedMaybeFinalizedWithRLock()
}

func (r *fsSegment) matchAllNotClosedMaybeFinalizedWithRLock() (postings.MutableList, error) {
	// NB(r): Not closed, but could be finalized (i.e. closed segment reader)
	// calling match field after this segment is finalized.
	if r.finalized {
		return nil, errReaderFinalized
	}

	pl := r.opts.PostingsListPool().Get()
	err := pl.AddRange(0, postings.ID(r.numDocs))
	if err != nil {
		return nil, err
	}

	return pl, nil
}

func (r *fsSegment) Doc(id postings.ID) (doc.Document, error) {
	r.RLock()
	defer r.RUnlock()
	if r.closed {
		return doc.Document{}, errReaderClosed
	}
	return r.docNotClosedMaybeFinalizedWithRLock(id)
}

func (r *fsSegment) docNotClosedMaybeFinalizedWithRLock(id postings.ID) (doc.Document, error) {
	// NB(r): Not closed, but could be finalized (i.e. closed segment reader)
	// calling match field after this segment is finalized.
	if r.finalized {
		return doc.Document{}, errReaderFinalized
	}

	// If using docs slice reader, return from the in memory slice reader
	if r.docsThirdPartyReader != nil {
		return r.docsThirdPartyReader.Read(id)
	}

	offset, err := r.docsIndexReader.Read(id)
	if err != nil {
		return doc.Document{}, err
	}

	return r.docsDataReader.Read(offset)
}

func (r *fsSegment) Docs(pl postings.List) (doc.Iterator, error) {
	r.RLock()
	defer r.RUnlock()
	if r.closed {
		return nil, errReaderClosed
	}
	return r.docsNotClosedMaybeFinalizedWithRLock(r, pl)
}

func (r *fsSegment) docsNotClosedMaybeFinalizedWithRLock(
	retriever index.DocRetriever,
	pl postings.List,
) (doc.Iterator, error) {
	// NB(r): Not closed, but could be finalized (i.e. closed segment reader)
	// calling match field after this segment is finalized.
	if r.finalized {
		return nil, errReaderFinalized
	}

	return index.NewIDDocIterator(retriever, pl.Iterator()), nil
}

func (r *fsSegment) AllDocs() (index.IDDocIterator, error) {
	r.RLock()
	defer r.RUnlock()
	if r.closed {
		return nil, errReaderClosed
	}
	return r.allDocsNotClosedMaybeFinalizedWithRLock(r)
}

func (r *fsSegment) allDocsNotClosedMaybeFinalizedWithRLock(
	retriever index.DocRetriever,
) (index.IDDocIterator, error) {
	// NB(r): Not closed, but could be finalized (i.e. closed segment reader)
	// calling match field after this segment is finalized.
	if r.finalized {
		return nil, errReaderFinalized
	}

	pi := postings.NewRangeIterator(0, postings.ID(r.numDocs))
	return index.NewIDDocIterator(retriever, pi), nil
}

func (r *fsSegment) retrievePostingsListWithRLock(postingsOffset uint64) (postings.List, error) {
	postingsBytes, err := r.retrieveBytesWithRLock(r.data.PostingsData.Bytes, postingsOffset)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve postings data: %v", err)
	}

	return pilosa.Unmarshal(postingsBytes)
}

func (r *fsSegment) retrieveTermsFSTWithRLock(field []byte) (*vellum.FST, bool, error) {
	termsFSTOffset, exists, err := r.fieldsFST.Get(field)
	if err != nil {
		return nil, false, err
	}

	if !exists {
		return nil, false, nil
	}

	termsFSTBytes, err := r.retrieveBytesWithRLock(r.data.FSTTermsData.Bytes, termsFSTOffset)
	if err != nil {
		return nil, false, fmt.Errorf("error while decoding terms fst: %v", err)
	}

	termsFST, err := vellum.Load(termsFSTBytes)
	if err != nil {
		return nil, false, fmt.Errorf("error while loading terms fst: %v", err)
	}

	return termsFST, true, nil
}

// retrieveTermsBytesWithRLock assumes the base []byte slice is a collection of
// (protobuf payload, proto payload size, fst payload, fst payload size, magicNumber) tuples;
// where all sizes/magicNumber are strictly uint64 (i.e. 8 bytes). It assumes the 8 bytes
// preceding the offset are the magicNumber, the 8 bytes before that are the fst payload size,
// and the `size` bytes before that are the payload, 8 bytes preceeding that are
// `proto payload size`, and the `proto payload size` bytes before that are the proto payload.
// It retrieves the payload while doing bounds checks to ensure no segfaults.
func (r *fsSegment) retrieveTermsBytesWithRLock(base []byte, offset uint64) (proto []byte, fst []byte, err error) {
	const sizeofUint64 = 8
	var (
		magicNumberEnd   = int64(offset) // to prevent underflows
		magicNumberStart = offset - sizeofUint64
	)
	if magicNumberEnd > int64(len(base)) || magicNumberStart < 0 {
		return nil, nil, fmt.Errorf("base bytes too small, length: %d, base-offset: %d", len(base), magicNumberEnd)
	}
	magicNumberBytes := base[magicNumberStart:magicNumberEnd]
	d := encoding.NewDecoder(magicNumberBytes)
	n, err := d.Uint64()
	if err != nil {
		return nil, nil, fmt.Errorf("error while decoding magicNumber: %v", err)
	}
	if n != uint64(magicNumber) {
		return nil, nil, fmt.Errorf("mismatch while decoding magicNumber: %d", n)
	}

	var (
		sizeEnd   = magicNumberStart
		sizeStart = sizeEnd - sizeofUint64
	)
	if sizeStart < 0 {
		return nil, nil, fmt.Errorf("base bytes too small, length: %d, size-offset: %d", len(base), sizeStart)
	}
	sizeBytes := base[sizeStart:sizeEnd]
	d.Reset(sizeBytes)
	size, err := d.Uint64()
	if err != nil {
		return nil, nil, fmt.Errorf("error while decoding size: %v", err)
	}

	var (
		payloadEnd   = sizeStart
		payloadStart = payloadEnd - size
	)
	if payloadStart < 0 {
		return nil, nil, fmt.Errorf("base bytes too small, length: %d, payload-start: %d, payload-size: %d",
			len(base), payloadStart, size)
	}

	var (
		fstBytes       = base[payloadStart:payloadEnd]
		protoSizeEnd   = payloadStart
		protoSizeStart = protoSizeEnd - sizeofUint64
	)
	if protoSizeStart < 0 {
		return nil, nil, fmt.Errorf("base bytes too small, length: %d, proto-size-offset: %d", len(base), protoSizeStart)
	}

	protoSizeBytes := base[protoSizeStart:protoSizeEnd]
	d.Reset(protoSizeBytes)
	protoSize, err := d.Uint64()
	if err != nil {
		return nil, nil, fmt.Errorf("error while decoding size: proto %v", err)
	}

	var (
		protoEnd   = protoSizeStart
		protoStart = protoEnd - protoSize
	)
	if protoStart < 0 {
		return nil, nil, fmt.Errorf("base bytes too small, length: %d, proto-start: %d", len(base), protoStart)
	}
	protoBytes := base[protoStart:protoEnd]

	return protoBytes, fstBytes, nil
}

// retrieveBytesWithRLock assumes the base []byte slice is a collection of (payload, size, magicNumber) triples,
// where size/magicNumber are strictly uint64 (i.e. 8 bytes). It assumes the 8 bytes preceding the offset
// are the magicNumber, the 8 bytes before that are the size, and the `size` bytes before that are the
// payload. It retrieves the payload while doing bounds checks to ensure no segfaults.
func (r *fsSegment) retrieveBytesWithRLock(base []byte, offset uint64) ([]byte, error) {
	const sizeofUint64 = 8
	var (
		magicNumberEnd   = int64(offset) // to prevent underflows
		magicNumberStart = offset - sizeofUint64
	)
	if magicNumberEnd > int64(len(base)) || magicNumberStart < 0 {
		return nil, fmt.Errorf("base bytes too small, length: %d, base-offset: %d", len(base), magicNumberEnd)
	}
	magicNumberBytes := base[magicNumberStart:magicNumberEnd]
	d := encoding.NewDecoder(magicNumberBytes)
	n, err := d.Uint64()
	if err != nil {
		return nil, fmt.Errorf("error while decoding magicNumber: %v", err)
	}
	if n != uint64(magicNumber) {
		return nil, fmt.Errorf("mismatch while decoding magicNumber: %d", n)
	}

	var (
		sizeEnd   = magicNumberStart
		sizeStart = sizeEnd - sizeofUint64
	)
	if sizeStart < 0 {
		return nil, fmt.Errorf("base bytes too small, length: %d, size-offset: %d", len(base), sizeStart)
	}
	sizeBytes := base[sizeStart:sizeEnd]
	d.Reset(sizeBytes)
	size, err := d.Uint64()
	if err != nil {
		return nil, fmt.Errorf("error while decoding size: %v", err)
	}

	var (
		payloadEnd   = sizeStart
		payloadStart = payloadEnd - size
	)
	if payloadStart < 0 {
		return nil, fmt.Errorf("base bytes too small, length: %d, payload-start: %d, payload-size: %d",
			len(base), payloadStart, size)
	}

	return base[payloadStart:payloadEnd], nil
}

var _ sgmt.Reader = (*fsSegmentReader)(nil)

// fsSegmentReader is not thread safe for use and relies on the underlying
// segment for synchronization.
type fsSegmentReader struct {
	closed        bool
	ctx           context.Context
	fsSegment     *fsSegment
	termsIterable *termsIterable
}

func newReader(
	fsSegment *fsSegment,
	opts Options,
) *fsSegmentReader {
	return &fsSegmentReader{
		ctx:       opts.ContextPool().Get(),
		fsSegment: fsSegment,
	}
}

func (sr *fsSegmentReader) Fields() (sgmt.FieldsIterator, error) {
	if sr.closed {
		return nil, errReaderClosed
	}

	iter := newFSTTermsIter()
	iter.reset(fstTermsIterOpts{
		seg:         sr.fsSegment,
		fst:         sr.fsSegment.fieldsFST,
		finalizeFST: false,
	})
	return iter, nil
}

func (sr *fsSegmentReader) ContainsField(field []byte) (bool, error) {
	if sr.closed {
		return false, errReaderClosed
	}

	sr.fsSegment.RLock()
	defer sr.fsSegment.RUnlock()
	if sr.fsSegment.finalized {
		return false, errReaderFinalized
	}

	return sr.fsSegment.fieldsFST.Contains(field)
}

func (sr *fsSegmentReader) Terms(field []byte) (sgmt.TermsIterator, error) {
	if sr.closed {
		return nil, errReaderClosed
	}
	if sr.termsIterable == nil {
		sr.termsIterable = newTermsIterable(sr.fsSegment)
	}
	sr.fsSegment.RLock()
	iter, err := sr.termsIterable.termsNotClosedMaybeFinalizedWithRLock(field)
	sr.fsSegment.RUnlock()
	return iter, err
}

func (sr *fsSegmentReader) MatchField(field []byte) (postings.List, error) {
	if sr.closed {
		return nil, errReaderClosed
	}
	// NB(r): We are allowed to call match field after Close called on
	// the segment but not after it is finalized.
	sr.fsSegment.RLock()
	pl, err := sr.fsSegment.matchFieldNotClosedMaybeFinalizedWithRLock(field)
	sr.fsSegment.RUnlock()
	return pl, err
}

func (sr *fsSegmentReader) MatchTerm(field []byte, term []byte) (postings.List, error) {
	if sr.closed {
		return nil, errReaderClosed
	}
	// NB(r): We are allowed to call match field after Close called on
	// the segment but not after it is finalized.
	sr.fsSegment.RLock()
	pl, err := sr.fsSegment.matchTermNotClosedMaybeFinalizedWithRLock(field, term)
	sr.fsSegment.RUnlock()
	return pl, err
}

func (sr *fsSegmentReader) MatchRegexp(
	field []byte,
	compiled index.CompiledRegex,
) (postings.List, error) {
	if sr.closed {
		return nil, errReaderClosed
	}
	// NB(r): We are allowed to call match field after Close called on
	// the segment but not after it is finalized.
	sr.fsSegment.RLock()
	pl, err := sr.fsSegment.matchRegexpNotClosedMaybeFinalizedWithRLock(field, compiled)
	sr.fsSegment.RUnlock()
	return pl, err
}

func (sr *fsSegmentReader) MatchAll() (postings.MutableList, error) {
	if sr.closed {
		return nil, errReaderClosed
	}
	// NB(r): We are allowed to call match field after Close called on
	// the segment but not after it is finalized.
	sr.fsSegment.RLock()
	pl, err := sr.fsSegment.matchAllNotClosedMaybeFinalizedWithRLock()
	sr.fsSegment.RUnlock()
	return pl, err
}

func (sr *fsSegmentReader) Doc(id postings.ID) (doc.Document, error) {
	if sr.closed {
		return doc.Document{}, errReaderClosed
	}
	// NB(r): We are allowed to call match field after Close called on
	// the segment but not after it is finalized.
	sr.fsSegment.RLock()
	pl, err := sr.fsSegment.docNotClosedMaybeFinalizedWithRLock(id)
	sr.fsSegment.RUnlock()
	return pl, err
}

func (sr *fsSegmentReader) Docs(pl postings.List) (doc.Iterator, error) {
	if sr.closed {
		return nil, errReaderClosed
	}
	// NB(r): We are allowed to call match field after Close called on
	// the segment but not after it is finalized.
	// Also make sure the doc retriever is the reader not the segment so that
	// is closed check is not performed and only the is finalized check.
	sr.fsSegment.RLock()
	iter, err := sr.fsSegment.docsNotClosedMaybeFinalizedWithRLock(sr, pl)
	sr.fsSegment.RUnlock()
	return iter, err
}

func (sr *fsSegmentReader) AllDocs() (index.IDDocIterator, error) {
	if sr.closed {
		return nil, errReaderClosed
	}
	// NB(r): We are allowed to call match field after Close called on
	// the segment but not after it is finalized.
	// Also make sure the doc retriever is the reader not the segment so that
	// is closed check is not performed and only the is finalized check.
	sr.fsSegment.RLock()
	iter, err := sr.fsSegment.allDocsNotClosedMaybeFinalizedWithRLock(sr)
	sr.fsSegment.RUnlock()
	return iter, err
}

func (sr *fsSegmentReader) Close() error {
	if sr.closed {
		return errReaderClosed
	}
	sr.closed = true
	// Close the context so that segment doesn't need to track this any longer.
	sr.ctx.Close()
	return nil
}
