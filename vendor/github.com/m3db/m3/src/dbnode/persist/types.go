// Copyright (c) 2016 Uber Technologies, Inc.
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
	"time"

	"github.com/m3db/m3/src/dbnode/namespace"
	"github.com/m3db/m3/src/dbnode/ts"
	"github.com/m3db/m3/src/m3ninx/doc"
	"github.com/m3db/m3/src/m3ninx/index/segment"
	idxpersist "github.com/m3db/m3/src/m3ninx/persist"
	"github.com/m3db/m3/src/x/ident"

	"github.com/pborman/uuid"
)

var (
	errReuseableTagIteratorRequired = errors.New("reuseable tags iterator is required")
)

// Metadata is metadata for a time series, it can
// have several underlying sources.
type Metadata struct {
	metadata doc.Document
	id       ident.ID
	tags     ident.Tags
	tagsIter ident.TagIterator
	opts     MetadataOptions
}

// MetadataOptions is options to use when creating metadata.
type MetadataOptions struct {
	FinalizeID          bool
	FinalizeTags        bool
	FinalizeTagIterator bool
}

// NewMetadata returns a new metadata struct from series metadata.
// Note: because doc.Document has no pools for finalization we do not
// take MetadataOptions here, in future if we have pools or
// some other shared options that Metadata needs we will add it to this
// constructor as well.
func NewMetadata(metadata doc.Document) Metadata {
	return Metadata{metadata: metadata}
}

// NewMetadataFromIDAndTags returns a new metadata struct from
// explicit ID and tags.
func NewMetadataFromIDAndTags(
	id ident.ID,
	tags ident.Tags,
	opts MetadataOptions,
) Metadata {
	return Metadata{
		id:   id,
		tags: tags,
		opts: opts,
	}
}

// NewMetadataFromIDAndTagIterator returns a new metadata struct from
// explicit ID and tag iterator.
func NewMetadataFromIDAndTagIterator(
	id ident.ID,
	tagsIter ident.TagIterator,
	opts MetadataOptions,
) Metadata {
	return Metadata{
		id:       id,
		tagsIter: tagsIter,
		opts:     opts,
	}
}

// BytesID returns the bytes ID of the series.
func (m Metadata) BytesID() []byte {
	if m.id != nil {
		return m.id.Bytes()
	}
	return m.metadata.ID
}

// ResetOrReturnProvidedTagIterator returns a tag iterator
// for the series, returning a direct ref to a provided tag
// iterator or using the reuseable tag iterator provided by the
// callsite if it needs to iterate over tags or fields.
func (m Metadata) ResetOrReturnProvidedTagIterator(
	reuseableTagsIterator ident.TagsIterator,
) (ident.TagIterator, error) {
	if reuseableTagsIterator == nil {
		// Always check to make sure callsites won't
		// get a bad allocation pattern of having
		// to create one here inline if the metadata
		// they are passing in suddenly changes from
		// tagsIter to tags or fields with metadata.
		return nil, errReuseableTagIteratorRequired
	}
	if m.tagsIter != nil {
		return m.tagsIter, nil
	}

	if len(m.tags.Values()) > 0 {
		reuseableTagsIterator.Reset(m.tags)
		return reuseableTagsIterator, reuseableTagsIterator.Err()
	}

	reuseableTagsIterator.ResetFields(m.metadata.Fields)
	return reuseableTagsIterator, reuseableTagsIterator.Err()
}

// Finalize will finalize any resources that requested
// to be finalized.
func (m Metadata) Finalize() {
	if m.opts.FinalizeID && m.id != nil {
		m.id.Finalize()
	}
	if m.opts.FinalizeTags && m.tags.Values() != nil {
		m.tags.Finalize()
	}
	if m.opts.FinalizeTagIterator && m.tagsIter != nil {
		m.tagsIter.Close()
	}
}

// DataFn is a function that persists a m3db segment for a given ID.
type DataFn func(metadata Metadata, segment ts.Segment, checksum uint32) error

// DataCloser is a function that performs cleanup after persisting the data
// blocks for a (shard, blockStart) combination.
type DataCloser func() error

// DeferCloser returns a DataCloser that persists the data checkpoint file when called.
type DeferCloser func() (DataCloser, error)

// PreparedDataPersist is an object that wraps holds a persist function and a closer.
type PreparedDataPersist struct {
	Persist    DataFn
	Close      DataCloser
	DeferClose DeferCloser
}

// CommitLogFiles represents a slice of commitlog files.
type CommitLogFiles []CommitLogFile

// Contains returns a boolean indicating whether the CommitLogFiles slice
// contains the provided CommitlogFile based on its path.
func (c CommitLogFiles) Contains(path string) bool {
	for _, f := range c {
		if f.FilePath == path {
			return true
		}
	}
	return false
}

// CommitLogFile represents a commit log file and its associated metadata.
type CommitLogFile struct {
	FilePath string
	Index    int64
}

// IndexFn is a function that persists a m3ninx MutableSegment.
type IndexFn func(segment.Builder) error

// IndexCloser is a function that performs cleanup after persisting the index data
// block for a (namespace, blockStart) combination and returns the corresponding
// immutable Segment.
type IndexCloser func() ([]segment.Segment, error)

// PreparedIndexPersist is an object that wraps holds a persist function and a closer.
type PreparedIndexPersist struct {
	Persist IndexFn
	Close   IndexCloser
}

// Manager manages the internals of persisting data onto storage layer.
type Manager interface {
	// StartFlushPersist begins a data flush for a set of shards.
	StartFlushPersist() (FlushPreparer, error)

	// StartSnapshotPersist begins a snapshot for a set of shards.
	StartSnapshotPersist(snapshotID uuid.UUID) (SnapshotPreparer, error)

	// StartIndexPersist begins a flush for index data.
	StartIndexPersist() (IndexFlush, error)

	Close()
}

// Preparer can generate a PreparedDataPersist object for writing data for
// a given (shard, blockstart) combination.
type Preparer interface {
	// Prepare prepares writing data for a given (shard, blockStart) combination,
	// returning a PreparedDataPersist object and any error encountered during
	// preparation if any.
	PrepareData(opts DataPrepareOptions) (PreparedDataPersist, error)
}

// FlushPreparer is a persist flush cycle, each shard and block start permutation needs
// to explicitly be prepared.
type FlushPreparer interface {
	Preparer

	// DoneFlush marks the data flush as complete.
	DoneFlush() error
}

// SnapshotPreparer is a persist snapshot cycle, each shard and block start permutation needs
// to explicitly be prepared.
type SnapshotPreparer interface {
	Preparer

	// DoneSnapshot marks the snapshot as complete.
	DoneSnapshot(snapshotUUID uuid.UUID, commitLogIdentifier CommitLogFile) error
}

// IndexFlush is a persist flush cycle, each namespace, block combination needs
// to explicitly be prepared.
type IndexFlush interface {
	// Prepare prepares writing data for a given ns/blockStart, returning a
	// PreparedIndexPersist object and any error encountered during
	// preparation if any.
	PrepareIndex(opts IndexPrepareOptions) (PreparedIndexPersist, error)

	// DoneIndex marks the index flush as complete.
	DoneIndex() error
}

// DataPrepareOptions is the options struct for the DataFlush's Prepare method.
// nolint: maligned
type DataPrepareOptions struct {
	NamespaceMetadata namespace.Metadata
	BlockStart        time.Time
	Shard             uint32
	// This volume index is only used when preparing for a flush fileset type.
	// When opening a snapshot, the new volume index is determined by looking
	// at what files exist on disk.
	VolumeIndex    int
	FileSetType    FileSetType
	DeleteIfExists bool
	// Snapshot options are applicable to snapshots (index yes, data yes)
	Snapshot DataPrepareSnapshotOptions
}

// IndexPrepareOptions is the options struct for the IndexFlush's Prepare method.
// nolint: maligned
type IndexPrepareOptions struct {
	NamespaceMetadata namespace.Metadata
	BlockStart        time.Time
	FileSetType       FileSetType
	Shards            map[uint32]struct{}
	IndexVolumeType   idxpersist.IndexVolumeType
}

// DataPrepareSnapshotOptions is the options struct for the Prepare method that contains
// information specific to read/writing snapshot files.
type DataPrepareSnapshotOptions struct {
	SnapshotTime time.Time
	SnapshotID   uuid.UUID
}

// FileSetType is an enum that indicates what type of files a fileset contains
type FileSetType int

func (f FileSetType) String() string {
	switch f {
	case FileSetFlushType:
		return "flush"
	case FileSetSnapshotType:
		return "snapshot"
	}

	return fmt.Sprintf("unknown: %d", f)
}

const (
	// FileSetFlushType indicates that the fileset files contain a complete flush
	FileSetFlushType FileSetType = iota
	// FileSetSnapshotType indicates that the fileset files contain a snapshot
	FileSetSnapshotType
)

// FileSetContentType is an enum that indicates what the contents of files a fileset contains
type FileSetContentType int

func (f FileSetContentType) String() string {
	switch f {
	case FileSetDataContentType:
		return "data"
	case FileSetIndexContentType:
		return "index"
	}
	return fmt.Sprintf("unknown: %d", f)
}

const (
	// FileSetDataContentType indicates that the fileset files contents is time series data
	FileSetDataContentType FileSetContentType = iota
	// FileSetIndexContentType indicates that the fileset files contain time series index metadata
	FileSetIndexContentType
)

// OnFlushNewSeriesEvent is the fields related to a flush of a new series.
type OnFlushNewSeriesEvent struct {
	Shard          uint32
	BlockStart     time.Time
	FirstWrite     time.Time
	SeriesMetadata doc.Document
}

// OnFlushSeries performs work on a per series level.
type OnFlushSeries interface {
	OnFlushNewSeries(OnFlushNewSeriesEvent) error
}

// NoOpColdFlushNamespace is a no-op impl of OnFlushSeries.
type NoOpColdFlushNamespace struct{}

// OnFlushNewSeries is a no-op.
func (n *NoOpColdFlushNamespace) OnFlushNewSeries(event OnFlushNewSeriesEvent) error {
	return nil
}

// Done is a no-op.
func (n *NoOpColdFlushNamespace) Done() error { return nil }
