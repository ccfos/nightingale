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

package builder

import (
	"errors"
	"fmt"
	"runtime"
	"sync"

	"github.com/m3db/m3/src/m3ninx/doc"
	"github.com/m3db/m3/src/m3ninx/index"
	"github.com/m3db/m3/src/m3ninx/index/segment"
	"github.com/m3db/m3/src/m3ninx/postings"
	"github.com/m3db/m3/src/m3ninx/util"

	"github.com/cespare/xxhash/v2"
	"github.com/twotwotwo/sorts"
)

var (
	errDocNotFound = errors.New("doc not found")
	errClosed      = errors.New("builder closed")
)

const (
	// Slightly buffer the work to avoid blocking main thread.
	indexQueueSize     = 2 << 9 // 1024
	entriesPerIndexJob = 32
)

var (
	globalIndexWorkers  = &indexWorkers{}
	fieldsMapSetOptions = fieldsMapSetUnsafeOptions{
		// Builder takes ownership of keys and docs so it's ok
		// to avoid copying and finalizing keys.
		NoCopyKey:     true,
		NoFinalizeKey: true,
	}
)

type indexWorkers struct {
	sync.RWMutex
	builders int
	queues   []chan indexJob
}

type indexJob struct {
	wg *sync.WaitGroup

	opts Options

	entries     [entriesPerIndexJob]indexJobEntry
	usedEntries int

	shard         int
	shardedFields *shardedFields

	batchErr *index.BatchPartialError
}

type indexJobEntry struct {
	id     postings.ID
	field  doc.Field
	docIdx int
}

func (w *indexWorkers) registerBuilder() {
	w.Lock()
	defer w.Unlock()

	preIncBuilders := w.builders
	w.builders++

	if preIncBuilders != 0 {
		return // Already initialized.
	}

	// Need to initialize structures, prepare all num CPU
	// worker queues, even if we don't use all of them.
	n := runtime.NumCPU()
	if cap(w.queues) == 0 {
		w.queues = make([]chan indexJob, 0, n)
	} else {
		// Reuse existing queues slice.
		w.queues = w.queues[:0]
	}

	// Start the workers.
	for i := 0; i < n; i++ {
		indexQueue := make(chan indexJob, indexQueueSize)
		w.queues = append(w.queues, indexQueue)
		go w.indexWorker(indexQueue)
	}
}

func (w *indexWorkers) indexWorker(indexQueue <-chan indexJob) {
	for job := range indexQueue {
		for i := 0; i < job.usedEntries; i++ {
			entry := job.entries[i]
			terms, ok := job.shardedFields.fields.ShardedGet(job.shard, entry.field.Name)
			if !ok {
				// NB(bodu): Check again within the lock to make sure we aren't making concurrent map writes.
				terms = newTerms(job.opts)
				job.shardedFields.fields.ShardedSetUnsafe(job.shard, entry.field.Name,
					terms, fieldsMapSetOptions)
			}

			// If empty field, track insertion of this key into the fields
			// collection for correct response when retrieving all fields.
			newField := terms.size() == 0
			// NB(bodu): Bulk of the cpu time during insertion is spent inside of terms.post().
			err := terms.post(entry.field.Value, entry.id)
			if err != nil {
				job.batchErr.AddWithLock(index.BatchError{Err: err, Idx: entry.docIdx})
			}
			if err == nil && newField {
				newEntry := uniqueField{
					field:        entry.field.Name,
					postingsList: terms.postingsListUnion,
				}
				job.shardedFields.uniqueFields[job.shard] =
					append(job.shardedFields.uniqueFields[job.shard], newEntry)
			}
		}

		job.wg.Done()
	}
}

func (w *indexWorkers) indexJob(job indexJob) {
	w.queues[job.shard] <- job
}

func (w *indexWorkers) unregisterBuilder() {
	w.Lock()
	defer w.Unlock()

	w.builders--

	if w.builders != 0 {
		return // Still have registered builders, cannot spin down yet.
	}

	// Close the workers.
	for i := range w.queues {
		close(w.queues[i])
		w.queues[i] = nil
	}
	w.queues = w.queues[:0]
}

type builderStatus struct {
	sync.RWMutex
	closed bool
}

type builder struct {
	opts      Options
	newUUIDFn util.NewUUIDFn

	batchSizeOne  index.Batch
	docs          []doc.Document
	idSet         *IDsMap
	shardedJobs   []indexJob
	shardedFields *shardedFields
	concurrency   int

	status builderStatus
}

type shardedFields struct {
	fields       *shardedFieldsMap
	uniqueFields [][]uniqueField
}

// NewBuilderFromDocuments returns a builder from documents, it is
// not thread safe and is optimized for insertion speed and a
// final build step when documents are indexed.
func NewBuilderFromDocuments(opts Options) (segment.CloseableDocumentsBuilder, error) {
	b := &builder{
		opts:      opts,
		newUUIDFn: opts.NewUUIDFn(),
		batchSizeOne: index.Batch{
			Docs: make([]doc.Document, 1),
		},
		idSet: NewIDsMap(IDsMapOptions{
			InitialSize: opts.InitialCapacity(),
		}),
		shardedFields: &shardedFields{},
	}
	// Indiciate we need to spin up workers if we haven't already.
	globalIndexWorkers.registerBuilder()
	b.SetIndexConcurrency(opts.Concurrency())
	return b, nil
}

func (b *builder) SetIndexConcurrency(value int) {
	b.status.Lock()
	defer b.status.Unlock()

	if b.concurrency == value {
		return // No-op
	}

	b.concurrency = value

	// Nothing to migrate, jobs only used during a batch insertion.
	b.shardedJobs = make([]indexJob, b.concurrency)

	// Take refs to existing fields to migrate.
	existingUniqueFields := b.shardedFields.uniqueFields
	existingFields := b.shardedFields.fields

	b.shardedFields.uniqueFields = make([][]uniqueField, 0, b.concurrency)
	b.shardedFields.fields = newShardedFieldsMap(b.concurrency, b.opts.InitialCapacity())

	for i := 0; i < b.concurrency; i++ {
		// Give each shard a fraction of the configured initial capacity.
		shardInitialCapacity := b.opts.InitialCapacity()
		if shardInitialCapacity > 0 {
			shardInitialCapacity /= b.concurrency
		}

		shardUniqueFields := make([]uniqueField, 0, shardInitialCapacity)
		b.shardedFields.uniqueFields =
			append(b.shardedFields.uniqueFields, shardUniqueFields)
	}

	// Migrate data from existing unique fields.
	if existingUniqueFields != nil {
		for _, fields := range existingUniqueFields {
			for _, field := range fields {
				// Calculate the new shard for the field.
				newShard := b.calculateShardWithRLock(field.field)

				// Append to the correct shard.
				b.shardedFields.uniqueFields[newShard] =
					append(b.shardedFields.uniqueFields[newShard], field)
			}
		}
	}

	// Migrate from fields.
	if existingFields != nil {
		for _, fields := range existingFields.data {
			for _, entry := range fields.Iter() {
				field := entry.Key()
				terms := entry.Value()

				// Calculate the new shard for the field.
				newShard := b.calculateShardWithRLock(field)

				// Set with new correct shard.
				b.shardedFields.fields.ShardedSetUnsafe(newShard, field,
					terms, fieldsMapSetOptions)
			}
		}
	}
}

func (b *builder) IndexConcurrency() int {
	b.status.RLock()
	defer b.status.RUnlock()

	return b.concurrency
}

func (b *builder) Reset() {
	b.status.Lock()
	defer b.status.Unlock()

	// Reset the documents slice.
	var empty doc.Document
	for i := range b.docs {
		b.docs[i] = empty
	}
	b.docs = b.docs[:0]

	// Remove all entries in the ID set.
	b.idSet.Reset()

	// Keep fields around, just reset the terms set for each one.
	b.shardedFields.fields.ResetTermsSets()

	// Reset the unique fields slice
	var emptyField uniqueField
	for i, shardUniqueFields := range b.shardedFields.uniqueFields {
		for i := range shardUniqueFields {
			shardUniqueFields[i] = emptyField
		}
		b.shardedFields.uniqueFields[i] = shardUniqueFields[:0]
	}
}

func (b *builder) Insert(d doc.Document) ([]byte, error) {
	b.status.Lock()
	defer b.status.Unlock()

	// Use a preallocated slice to make insert able to avoid alloc
	// a slice to call insert batch with.
	b.batchSizeOne.Docs[0] = d
	err := b.insertBatchWithLock(b.batchSizeOne)
	if err != nil {
		if errs := err.Errs(); len(errs) == 1 {
			// Return concrete error instead of the batch partial error.
			return nil, errs[0].Err
		}
		// Fallback to returning batch partial error if not what we expect.
		return nil, err
	}
	last := b.docs[len(b.docs)-1]
	return last.ID, nil
}

func (b *builder) InsertBatch(batch index.Batch) error {
	b.status.Lock()
	defer b.status.Unlock()

	if b.status.closed {
		return errClosed
	}

	// NB(r): This switch is required or else *index.BatchPartialError
	// is returned as a non-nil wrapped "error" even though it is not
	// an error and underlying error is nil.
	if err := b.insertBatchWithLock(batch); err != nil {
		return err
	}
	return nil
}

func (b *builder) resetShardedJobs() {
	// Reset sharded jobs using memset optimization.
	var jobZeroed indexJob
	for i := range b.shardedJobs {
		b.shardedJobs[i] = jobZeroed
	}
}

func (b *builder) insertBatchWithLock(batch index.Batch) *index.BatchPartialError {
	// NB(r): This is all kept in a single method to make the
	// insertion path avoid too much function call overhead.
	wg := &sync.WaitGroup{}
	batchErr := index.NewBatchPartialError()

	// Reset shared resources and at cleanup too to remove refs.
	b.resetShardedJobs()
	defer b.resetShardedJobs()

	// Enqueue docs for indexing.
	for i, d := range batch.Docs {
		// Validate doc
		if err := d.Validate(); err != nil {
			batchErr.Add(index.BatchError{Err: err, Idx: i})
			continue
		}

		// Generate ID if needed.
		if !d.HasID() {
			id, err := b.newUUIDFn()
			if err != nil {
				batchErr.Add(index.BatchError{Err: err, Idx: i})
				continue
			}

			d.ID = id

			// Update the document in the batch since we added an ID to it.
			batch.Docs[i] = d
		}

		// Avoid duplicates.
		if _, ok := b.idSet.Get(d.ID); ok {
			batchErr.Add(index.BatchError{Err: index.ErrDuplicateID, Idx: i})
			continue
		}

		// Write to document set.
		b.idSet.SetUnsafe(d.ID, struct{}{}, IDsMapSetUnsafeOptions{
			NoCopyKey:     true,
			NoFinalizeKey: true,
		})

		// Every new document just gets the next available id.
		postingsListID := len(b.docs)
		b.docs = append(b.docs, d)

		// Index the terms.
		for _, f := range d.Fields {
			b.queueIndexJobEntryWithLock(wg, postings.ID(postingsListID), f, i, batchErr)
		}
		b.queueIndexJobEntryWithLock(wg, postings.ID(postingsListID), doc.Field{
			Name:  doc.IDReservedFieldName,
			Value: d.ID,
		}, i, batchErr)
	}

	// Enqueue any partially filled sharded jobs.
	for shard := 0; shard < b.concurrency; shard++ {
		if b.shardedJobs[shard].usedEntries > 0 {
			b.flushShardedIndexJobWithLock(shard, wg, batchErr)
		}
	}

	// Wait for all the concurrent indexing jobs to finish.
	wg.Wait()

	if !batchErr.IsEmpty() {
		return batchErr
	}
	return nil
}

func (b *builder) queueIndexJobEntryWithLock(
	wg *sync.WaitGroup,
	id postings.ID,
	field doc.Field,
	docIdx int,
	batchErr *index.BatchPartialError,
) {
	shard := b.calculateShardWithRLock(field.Name)
	entryIndex := b.shardedJobs[shard].usedEntries
	b.shardedJobs[shard].usedEntries++
	b.shardedJobs[shard].entries[entryIndex].id = id
	b.shardedJobs[shard].entries[entryIndex].field = field
	b.shardedJobs[shard].entries[entryIndex].docIdx = docIdx

	numEntries := b.shardedJobs[shard].usedEntries
	if numEntries != entriesPerIndexJob {
		return
	}

	// Ready to flush this job since all entries are used.
	b.flushShardedIndexJobWithLock(shard, wg, batchErr)

	// Reset for reuse.
	b.shardedJobs[shard] = indexJob{}
}

func (b *builder) flushShardedIndexJobWithLock(
	shard int,
	wg *sync.WaitGroup,
	batchErr *index.BatchPartialError,
) {
	// Set common fields.
	b.shardedJobs[shard].shard = shard
	b.shardedJobs[shard].wg = wg
	b.shardedJobs[shard].batchErr = batchErr
	b.shardedJobs[shard].shardedFields = b.shardedFields
	b.shardedJobs[shard].opts = b.opts

	// Enqueue job.
	wg.Add(1)
	globalIndexWorkers.indexJob(b.shardedJobs[shard])
}

func (b *builder) calculateShardWithRLock(field []byte) int {
	return int(xxhash.Sum64(field) % uint64(b.concurrency))
}

func (b *builder) AllDocs() (index.IDDocIterator, error) {
	b.status.RLock()
	defer b.status.RUnlock()

	rangeIter := postings.NewRangeIterator(0, postings.ID(len(b.docs)))
	return index.NewIDDocIterator(b, rangeIter), nil
}

func (b *builder) Doc(id postings.ID) (doc.Document, error) {
	b.status.RLock()
	defer b.status.RUnlock()

	idx := int(id)
	if idx < 0 || idx >= len(b.docs) {
		return doc.Document{}, errDocNotFound
	}

	return b.docs[idx], nil
}

func (b *builder) Docs() []doc.Document {
	b.status.RLock()
	defer b.status.RUnlock()

	return b.docs
}

func (b *builder) FieldsIterable() segment.FieldsPostingsListIterable {
	return b
}

func (b *builder) TermsIterable() segment.TermsIterable {
	return b
}

func (b *builder) FieldsPostingsList() (segment.FieldsPostingsListIterator, error) {
	// NB(r): Need write lock since sort in newOrderedFieldsPostingsListIter
	// and SetConcurrency causes sharded fields to change.
	b.status.Lock()
	defer b.status.Unlock()

	return newOrderedFieldsPostingsListIter(b.shardedFields.uniqueFields), nil
}

func (b *builder) Terms(field []byte) (segment.TermsIterator, error) {
	// NB(r): Need write lock since sort if required below
	// and SetConcurrency causes sharded fields to change.
	b.status.Lock()
	defer b.status.Unlock()

	shard := b.calculateShardWithRLock(field)
	terms, ok := b.shardedFields.fields.ShardedGet(shard, field)
	if !ok {
		return nil, fmt.Errorf("field not found: %s", string(field))
	}

	// NB(r): Ensure always sorted so can be used to build an FST which
	// requires in order insertion.
	terms.sortIfRequired()

	return newTermsIter(terms.uniqueTerms), nil
}

func (b *builder) Close() error {
	b.status.Lock()
	defer b.status.Unlock()

	b.status.closed = true
	// Indiciate we could possibly spin down workers if no builders open.
	globalIndexWorkers.unregisterBuilder()
	return nil
}

var (
	sortConcurrencyLock sync.RWMutex
)

// SetSortConcurrency sets the sort concurrency for when
// building segments, unfortunately this must be set globally
// since github.com/twotwotwo/sorts does not provide an
// ability to set parallelism on call to sort.
func SetSortConcurrency(value int) {
	sortConcurrencyLock.Lock()
	sorts.MaxProcs = value
	sortConcurrencyLock.Unlock()
}
