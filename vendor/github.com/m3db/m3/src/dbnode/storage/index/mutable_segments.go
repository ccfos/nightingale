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

package index

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/m3db/m3/src/dbnode/namespace"
	"github.com/m3db/m3/src/dbnode/storage/index/compaction"
	"github.com/m3db/m3/src/dbnode/storage/index/segments"
	m3ninxindex "github.com/m3db/m3/src/m3ninx/index"
	"github.com/m3db/m3/src/m3ninx/index/segment"
	"github.com/m3db/m3/src/m3ninx/index/segment/builder"
	"github.com/m3db/m3/src/m3ninx/index/segment/fst"
	xclose "github.com/m3db/m3/src/x/close"
	"github.com/m3db/m3/src/x/context"
	"github.com/m3db/m3/src/x/instrument"
	"github.com/m3db/m3/src/x/mmap"

	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

var (
	errUnableToWriteBlockConcurrent            = errors.New("unable to write, index block is being written to already")
	errMutableSegmentsAlreadyClosed            = errors.New("mutable segments already closed")
	errForegroundCompactorNoPlan               = errors.New("index foreground compactor failed to generate a plan")
	errForegroundCompactorBadPlanFirstTask     = errors.New("index foreground compactor generated plan without mutable segment in first task")
	errForegroundCompactorBadPlanSecondaryTask = errors.New("index foreground compactor generated plan with mutable segment a secondary task")
)

type mutableSegmentsState uint

const (
	mutableSegmentsStateOpen   mutableSegmentsState = iota
	mutableSegmentsStateClosed mutableSegmentsState = iota
)

// nolint: maligned
type mutableSegments struct {
	sync.RWMutex

	state mutableSegmentsState

	foregroundSegments []*readableSeg
	backgroundSegments []*readableSeg

	compact                  mutableSegmentsCompact
	blockStart               time.Time
	blockOpts                BlockOptions
	opts                     Options
	iopts                    instrument.Options
	optsListener             xclose.SimpleCloser
	writeIndexingConcurrency int

	metrics mutableSegmentsMetrics
	logger  *zap.Logger
}

type mutableSegmentsMetrics struct {
	foregroundCompactionPlanRunLatency tally.Timer
	foregroundCompactionTaskRunLatency tally.Timer
	backgroundCompactionPlanRunLatency tally.Timer
	backgroundCompactionTaskRunLatency tally.Timer
}

func newMutableSegmentsMetrics(s tally.Scope) mutableSegmentsMetrics {
	foregroundScope := s.Tagged(map[string]string{"compaction-type": "foreground"})
	backgroundScope := s.Tagged(map[string]string{"compaction-type": "background"})
	return mutableSegmentsMetrics{
		foregroundCompactionPlanRunLatency: foregroundScope.Timer("compaction-plan-run-latency"),
		foregroundCompactionTaskRunLatency: foregroundScope.Timer("compaction-task-run-latency"),
		backgroundCompactionPlanRunLatency: backgroundScope.Timer("compaction-plan-run-latency"),
		backgroundCompactionTaskRunLatency: backgroundScope.Timer("compaction-task-run-latency"),
	}
}

// NewBlock returns a new Block, representing a complete reverse index for the
// duration of time specified. It is backed by one or more segments.
func newMutableSegments(
	blockStart time.Time,
	opts Options,
	blockOpts BlockOptions,
	namespaceRuntimeOptsMgr namespace.RuntimeOptionsManager,
	iopts instrument.Options,
) *mutableSegments {
	m := &mutableSegments{
		blockStart: blockStart,
		opts:       opts,
		blockOpts:  blockOpts,
		iopts:      iopts,
		metrics:    newMutableSegmentsMetrics(iopts.MetricsScope()),
		logger:     iopts.Logger(),
	}
	m.optsListener = namespaceRuntimeOptsMgr.RegisterListener(m)
	return m
}

func (m *mutableSegments) SetNamespaceRuntimeOptions(opts namespace.RuntimeOptions) {
	m.Lock()
	// Update current runtime opts for segment builders created in future.
	perCPUFraction := opts.WriteIndexingPerCPUConcurrencyOrDefault()
	cpus := math.Ceil(perCPUFraction * float64(runtime.NumCPU()))
	m.writeIndexingConcurrency = int(math.Max(1, cpus))
	segmentBuilder := m.compact.segmentBuilder
	m.Unlock()

	// Reset any existing segment builder to new concurrency, do this
	// out of the lock since builder can be used for foreground compaction
	// outside the lock and does it's own locking.
	if segmentBuilder != nil {
		segmentBuilder.SetIndexConcurrency(m.writeIndexingConcurrency)
	}

	// Set the global concurrency control we have (we may need to fork
	// github.com/twotwotwo/sorts to control this on a per segment builder
	// basis).
	builder.SetSortConcurrency(m.writeIndexingConcurrency)
}

func (m *mutableSegments) WriteBatch(inserts *WriteBatch) error {
	m.Lock()
	if m.state == mutableSegmentsStateClosed {
		return errMutableSegmentsAlreadyClosed
	}

	if m.compact.compactingForeground {
		m.Unlock()
		return errUnableToWriteBlockConcurrent
	}

	// Lazily allocate the segment builder and compactors.
	err := m.compact.allocLazyBuilderAndCompactorsWithLock(m.writeIndexingConcurrency,
		m.blockOpts, m.opts)
	if err != nil {
		m.Unlock()
		return err
	}

	m.compact.compactingForeground = true
	builder := m.compact.segmentBuilder
	m.Unlock()

	defer func() {
		m.Lock()
		m.compact.compactingForeground = false
		m.cleanupForegroundCompactWithLock()
		m.Unlock()
	}()

	builder.Reset()
	insertResultErr := builder.InsertBatch(m3ninxindex.Batch{
		Docs:                inserts.PendingDocs(),
		AllowPartialUpdates: true,
	})
	if len(builder.Docs()) == 0 {
		// No inserts, no need to compact.
		return insertResultErr
	}

	// We inserted some documents, need to compact immediately into a
	// foreground segment from the segment builder before we can serve reads
	// from an FST segment.
	err = m.foregroundCompactWithBuilder(builder)
	if err != nil {
		return err
	}

	// Return result from the original insertion since compaction was successful.
	return insertResultErr
}

func (m *mutableSegments) AddReaders(readers []segment.Reader) ([]segment.Reader, error) {
	m.RLock()
	defer m.RUnlock()

	var err error
	readers, err = m.addReadersWithLock(m.foregroundSegments, readers)
	if err != nil {
		return nil, err
	}

	readers, err = m.addReadersWithLock(m.backgroundSegments, readers)
	if err != nil {
		return nil, err
	}

	return readers, nil
}

func (m *mutableSegments) addReadersWithLock(src []*readableSeg, dst []segment.Reader) ([]segment.Reader, error) {
	for _, seg := range src {
		reader, err := seg.Segment().Reader()
		if err != nil {
			return nil, err
		}
		dst = append(dst, reader)
	}
	return dst, nil
}

func (m *mutableSegments) Len() int {
	m.RLock()
	defer m.RUnlock()

	return len(m.foregroundSegments) + len(m.backgroundSegments)
}

func (m *mutableSegments) MemorySegmentsData(ctx context.Context) ([]fst.SegmentData, error) {
	m.RLock()
	defer m.RUnlock()

	// NB(r): This is for debug operations, do not bother about allocations.
	var results []fst.SegmentData
	for _, segs := range [][]*readableSeg{
		m.foregroundSegments,
		m.backgroundSegments,
	} {
		for _, seg := range segs {
			fstSegment, ok := seg.Segment().(fst.Segment)
			if !ok {
				return nil, fmt.Errorf("segment not fst segment: created=%v", seg.createdAt)
			}

			segmentData, err := fstSegment.SegmentData(ctx)
			if err != nil {
				return nil, err
			}

			results = append(results, segmentData)
		}
	}
	return results, nil
}

func (m *mutableSegments) NeedsEviction() bool {
	m.RLock()
	defer m.RUnlock()

	var needsEviction bool
	for _, seg := range m.foregroundSegments {
		needsEviction = needsEviction || seg.Segment().Size() > 0
	}
	for _, seg := range m.backgroundSegments {
		needsEviction = needsEviction || seg.Segment().Size() > 0
	}
	return needsEviction
}

func (m *mutableSegments) NumSegmentsAndDocs() (int64, int64) {
	m.RLock()
	defer m.RUnlock()

	var (
		numSegments, numDocs int64
	)
	for _, seg := range m.foregroundSegments {
		numSegments++
		numDocs += seg.Segment().Size()
	}
	for _, seg := range m.backgroundSegments {
		numSegments++
		numDocs += seg.Segment().Size()
	}
	return numSegments, numDocs
}

func (m *mutableSegments) Stats(reporter BlockStatsReporter) {
	m.RLock()
	defer m.RUnlock()

	for _, seg := range m.foregroundSegments {
		_, mutable := seg.Segment().(segment.MutableSegment)
		reporter.ReportSegmentStats(BlockSegmentStats{
			Type:    ActiveForegroundSegment,
			Mutable: mutable,
			Age:     seg.Age(),
			Size:    seg.Segment().Size(),
		})
	}
	for _, seg := range m.backgroundSegments {
		_, mutable := seg.Segment().(segment.MutableSegment)
		reporter.ReportSegmentStats(BlockSegmentStats{
			Type:    ActiveBackgroundSegment,
			Mutable: mutable,
			Age:     seg.Age(),
			Size:    seg.Segment().Size(),
		})
	}

	reporter.ReportIndexingStats(BlockIndexingStats{
		IndexConcurrency: m.writeIndexingConcurrency,
	})
}

func (m *mutableSegments) Close() {
	m.Lock()
	defer m.Unlock()
	m.state = mutableSegmentsStateClosed
	m.cleanupCompactWithLock()
	m.optsListener.Close()
}

func (m *mutableSegments) maybeBackgroundCompactWithLock() {
	if m.compact.compactingBackground {
		return
	}

	// Create a logical plan.
	segs := make([]compaction.Segment, 0, len(m.backgroundSegments))
	for _, seg := range m.backgroundSegments {
		segs = append(segs, compaction.Segment{
			Age:     seg.Age(),
			Size:    seg.Segment().Size(),
			Type:    segments.FSTType,
			Segment: seg.Segment(),
		})
	}

	plan, err := compaction.NewPlan(segs, m.opts.BackgroundCompactionPlannerOptions())
	if err != nil {
		instrument.EmitAndLogInvariantViolation(m.iopts, func(l *zap.Logger) {
			l.Error("index background compaction plan error", zap.Error(err))
		})
		return
	}

	if len(plan.Tasks) == 0 {
		return
	}

	// Kick off compaction.
	m.compact.compactingBackground = true
	go func() {
		m.backgroundCompactWithPlan(plan)

		m.Lock()
		m.compact.compactingBackground = false
		m.cleanupBackgroundCompactWithLock()
		m.Unlock()
	}()
}

func (m *mutableSegments) shouldEvictCompactedSegmentsWithLock() bool {
	return m.state == mutableSegmentsStateClosed
}

func (m *mutableSegments) cleanupBackgroundCompactWithLock() {
	if m.state == mutableSegmentsStateOpen {
		// See if we need to trigger another compaction.
		m.maybeBackgroundCompactWithLock()
		return
	}

	// Check if need to close all the compacted segments due to
	// mutableSegments being closed.
	if !m.shouldEvictCompactedSegmentsWithLock() {
		return
	}

	// Close compacted segments.
	m.closeCompactedSegmentsWithLock(m.backgroundSegments)
	m.backgroundSegments = nil

	// Free compactor resources.
	if m.compact.backgroundCompactor == nil {
		return
	}

	if err := m.compact.backgroundCompactor.Close(); err != nil {
		instrument.EmitAndLogInvariantViolation(m.iopts, func(l *zap.Logger) {
			l.Error("error closing index block background compactor", zap.Error(err))
		})
	}
	m.compact.backgroundCompactor = nil
}

func (m *mutableSegments) closeCompactedSegmentsWithLock(segments []*readableSeg) {
	for _, seg := range segments {
		err := seg.Segment().Close()
		if err != nil {
			instrument.EmitAndLogInvariantViolation(m.iopts, func(l *zap.Logger) {
				l.Error("could not close compacted segment", zap.Error(err))
			})
		}
	}
}

func (m *mutableSegments) backgroundCompactWithPlan(plan *compaction.Plan) {
	sw := m.metrics.backgroundCompactionPlanRunLatency.Start()
	defer sw.Stop()

	n := m.compact.numBackground
	m.compact.numBackground++

	logger := m.logger.With(
		zap.Time("blockStart", m.blockStart),
		zap.Int("numBackgroundCompaction", n),
	)
	log := n%compactDebugLogEvery == 0
	if log {
		for i, task := range plan.Tasks {
			summary := task.Summary()
			logger.Debug("planned background compaction task",
				zap.Int("task", i),
				zap.Int("numMutable", summary.NumMutable),
				zap.Int("numFST", summary.NumFST),
				zap.Stringer("cumulativeMutableAge", summary.CumulativeMutableAge),
				zap.Int64("cumulativeSize", summary.CumulativeSize),
			)
		}
	}

	for i, task := range plan.Tasks {
		err := m.backgroundCompactWithTask(task, log,
			logger.With(zap.Int("task", i)))
		if err != nil {
			instrument.EmitAndLogInvariantViolation(m.iopts, func(l *zap.Logger) {
				l.Error("error compacting segments", zap.Error(err))
			})
			return
		}
	}
}

func (m *mutableSegments) backgroundCompactWithTask(
	task compaction.Task,
	log bool,
	logger *zap.Logger,
) error {
	if log {
		logger.Debug("start compaction task")
	}

	segments := make([]segment.Segment, 0, len(task.Segments))
	for _, seg := range task.Segments {
		segments = append(segments, seg.Segment)
	}

	start := time.Now()
	compacted, err := m.compact.backgroundCompactor.Compact(segments, mmap.ReporterOptions{
		Context: mmap.Context{
			Name: mmapIndexBlockName,
		},
		Reporter: m.opts.MmapReporter(),
	})
	took := time.Since(start)
	m.metrics.backgroundCompactionTaskRunLatency.Record(took)

	if log {
		logger.Debug("done compaction task", zap.Duration("took", took))
	}

	if err != nil {
		return err
	}

	// Add a read through cache for repeated expensive queries against
	// background compacted segments since they can live for quite some
	// time and accrue a large set of documents.
	if immSeg, ok := compacted.(segment.ImmutableSegment); ok {
		var (
			plCache         = m.opts.PostingsListCache()
			readThroughOpts = m.opts.ReadThroughSegmentOptions()
		)
		compacted = NewReadThroughSegment(immSeg, plCache, readThroughOpts)
	}

	// Rotate out the replaced frozen segments and add the compacted one.
	m.Lock()
	defer m.Unlock()

	result := m.addCompactedSegmentFromSegmentsWithLock(m.backgroundSegments,
		segments, compacted)
	m.backgroundSegments = result

	return nil
}

func (m *mutableSegments) addCompactedSegmentFromSegmentsWithLock(
	current []*readableSeg,
	segmentsJustCompacted []segment.Segment,
	compacted segment.Segment,
) []*readableSeg {
	result := make([]*readableSeg, 0, len(current))
	for _, existing := range current {
		keepCurr := true
		for _, seg := range segmentsJustCompacted {
			if existing.Segment() == seg {
				// Do not keep this one, it was compacted just then.
				keepCurr = false
				break
			}
		}

		if keepCurr {
			result = append(result, existing)
			continue
		}

		err := existing.Segment().Close()
		if err != nil {
			// Already compacted, not much we can do about not closing it.
			instrument.EmitAndLogInvariantViolation(m.iopts, func(l *zap.Logger) {
				l.Error("unable to close compacted block", zap.Error(err))
			})
		}
	}

	// Return all the ones we kept plus the new compacted segment
	return append(result, newReadableSeg(compacted, m.opts))
}

func (m *mutableSegments) foregroundCompactWithBuilder(builder segment.DocumentsBuilder) error {
	// We inserted some documents, need to compact immediately into a
	// foreground segment.
	m.Lock()
	foregroundSegments := m.foregroundSegments
	m.Unlock()

	segs := make([]compaction.Segment, 0, len(foregroundSegments)+1)
	segs = append(segs, compaction.Segment{
		Age:     0,
		Size:    int64(len(builder.Docs())),
		Type:    segments.MutableType,
		Builder: builder,
	})
	for _, seg := range foregroundSegments {
		segs = append(segs, compaction.Segment{
			Age:     seg.Age(),
			Size:    seg.Segment().Size(),
			Type:    segments.FSTType,
			Segment: seg.Segment(),
		})
	}

	plan, err := compaction.NewPlan(segs, m.opts.ForegroundCompactionPlannerOptions())
	if err != nil {
		return err
	}

	// Check plan
	if len(plan.Tasks) == 0 {
		// Should always generate a task when a mutable builder is passed to planner
		return errForegroundCompactorNoPlan
	}
	if taskNumBuilders(plan.Tasks[0]) != 1 {
		// First task of plan must include the builder, so we can avoid resetting it
		// for the first task, but then safely reset it in consequent tasks
		return errForegroundCompactorBadPlanFirstTask
	}

	// Move any unused segments to the background.
	m.Lock()
	m.maybeMoveForegroundSegmentsToBackgroundWithLock(plan.UnusedSegments)
	m.Unlock()

	n := m.compact.numForeground
	m.compact.numForeground++

	logger := m.logger.With(
		zap.Time("blockStart", m.blockStart),
		zap.Int("numForegroundCompaction", n),
	)
	log := n%compactDebugLogEvery == 0
	if log {
		for i, task := range plan.Tasks {
			summary := task.Summary()
			logger.Debug("planned foreground compaction task",
				zap.Int("task", i),
				zap.Int("numMutable", summary.NumMutable),
				zap.Int("numFST", summary.NumFST),
				zap.Duration("cumulativeMutableAge", summary.CumulativeMutableAge),
				zap.Int64("cumulativeSize", summary.CumulativeSize),
			)
		}
	}

	// Run the plan.
	sw := m.metrics.foregroundCompactionPlanRunLatency.Start()
	defer sw.Stop()

	// Run the first task, without resetting the builder.
	if err := m.foregroundCompactWithTask(
		builder, plan.Tasks[0],
		log, logger.With(zap.Int("task", 0)),
	); err != nil {
		return err
	}

	// Now run each consequent task, resetting the builder each time since
	// the results from the builder have already been compacted in the first
	// task.
	for i := 1; i < len(plan.Tasks); i++ {
		task := plan.Tasks[i]
		if taskNumBuilders(task) > 0 {
			// Only the first task should compact the builder
			return errForegroundCompactorBadPlanSecondaryTask
		}
		// Now use the builder after resetting it.
		builder.Reset()
		if err := m.foregroundCompactWithTask(
			builder, task,
			log, logger.With(zap.Int("task", i)),
		); err != nil {
			return err
		}
	}

	return nil
}

func (m *mutableSegments) maybeMoveForegroundSegmentsToBackgroundWithLock(
	segments []compaction.Segment,
) {
	if len(segments) == 0 {
		return
	}
	if m.compact.backgroundCompactor == nil {
		// No longer performing background compaction due to evict/close.
		return
	}

	m.logger.Debug("moving segments from foreground to background",
		zap.Int("numSegments", len(segments)))

	// If background compaction is still active, then we move any unused
	// foreground segments into the background so that they might be
	// compacted by the background compactor at some point.
	i := 0
	for _, currForeground := range m.foregroundSegments {
		movedToBackground := false
		for _, seg := range segments {
			if currForeground.Segment() == seg.Segment {
				m.backgroundSegments = append(m.backgroundSegments, currForeground)
				movedToBackground = true
				break
			}
		}
		if movedToBackground {
			continue // No need to keep this segment, we moved it.
		}

		m.foregroundSegments[i] = currForeground
		i++
	}

	m.foregroundSegments = m.foregroundSegments[:i]

	// Potentially kick off a background compaction.
	m.maybeBackgroundCompactWithLock()
}

func (m *mutableSegments) foregroundCompactWithTask(
	builder segment.DocumentsBuilder,
	task compaction.Task,
	log bool,
	logger *zap.Logger,
) error {
	if log {
		logger.Debug("start compaction task")
	}

	segments := make([]segment.Segment, 0, len(task.Segments))
	for _, seg := range task.Segments {
		if seg.Segment == nil {
			continue // This means the builder is being used.
		}
		segments = append(segments, seg.Segment)
	}

	start := time.Now()
	compacted, err := m.compact.foregroundCompactor.CompactUsingBuilder(builder, segments, mmap.ReporterOptions{
		Context: mmap.Context{
			Name: mmapIndexBlockName,
		},
		Reporter: m.opts.MmapReporter(),
	})
	took := time.Since(start)
	m.metrics.foregroundCompactionTaskRunLatency.Record(took)

	if log {
		logger.Debug("done compaction task", zap.Duration("took", took))
	}

	if err != nil {
		return err
	}

	// Rotate in the ones we just compacted.
	m.Lock()
	defer m.Unlock()

	result := m.addCompactedSegmentFromSegmentsWithLock(m.foregroundSegments,
		segments, compacted)
	m.foregroundSegments = result

	return nil
}

func (m *mutableSegments) cleanupForegroundCompactWithLock() {
	// Check if need to close all the compacted segments due to
	// mutableSegments being closed.
	if !m.shouldEvictCompactedSegmentsWithLock() {
		return
	}

	// Close compacted segments.
	m.closeCompactedSegmentsWithLock(m.foregroundSegments)
	m.foregroundSegments = nil

	// Free compactor resources.
	if m.compact.foregroundCompactor != nil {
		if err := m.compact.foregroundCompactor.Close(); err != nil {
			instrument.EmitAndLogInvariantViolation(m.iopts, func(l *zap.Logger) {
				l.Error("error closing index block foreground compactor", zap.Error(err))
			})
		}
		m.compact.foregroundCompactor = nil
	}

	// Free segment builder resources.
	if m.compact.segmentBuilder != nil {
		if err := m.compact.segmentBuilder.Close(); err != nil {
			instrument.EmitAndLogInvariantViolation(m.iopts, func(l *zap.Logger) {
				l.Error("error closing index block segment builder", zap.Error(err))
			})
		}
		m.compact.segmentBuilder = nil
	}
}
func (m *mutableSegments) cleanupCompactWithLock() {
	// If not compacting, trigger a cleanup so that all frozen segments get
	// closed, otherwise after the current running compaction the compacted
	// segments will get closed.
	if !m.compact.compactingForeground {
		m.cleanupForegroundCompactWithLock()
	}
	if !m.compact.compactingBackground {
		m.cleanupBackgroundCompactWithLock()
	}
}

// mutableSegmentsCompact has several lazily allocated compaction components.
type mutableSegmentsCompact struct {
	segmentBuilder       segment.CloseableDocumentsBuilder
	foregroundCompactor  *compaction.Compactor
	backgroundCompactor  *compaction.Compactor
	compactingForeground bool
	compactingBackground bool
	numForeground        int
	numBackground        int
}

func (m *mutableSegmentsCompact) allocLazyBuilderAndCompactorsWithLock(
	concurrency int,
	blockOpts BlockOptions,
	opts Options,
) error {
	var (
		err      error
		docsPool = opts.DocumentArrayPool()
	)
	if m.segmentBuilder == nil {
		builderOpts := opts.SegmentBuilderOptions().
			SetConcurrency(concurrency)

		m.segmentBuilder, err = builder.NewBuilderFromDocuments(builderOpts)
		if err != nil {
			return err
		}
	}

	if m.foregroundCompactor == nil {
		m.foregroundCompactor, err = compaction.NewCompactor(docsPool,
			DocumentArrayPoolCapacity,
			opts.SegmentBuilderOptions(),
			opts.FSTSegmentOptions(),
			compaction.CompactorOptions{
				FSTWriterOptions: &fst.WriterOptions{
					// DisableRegistry is set to true to trade a larger FST size
					// for a faster FST compaction since we want to reduce the end
					// to end latency for time to first index a metric.
					DisableRegistry: true,
				},
				MmapDocsData: blockOpts.ForegroundCompactorMmapDocsData,
			})
		if err != nil {
			return err
		}
	}

	if m.backgroundCompactor == nil {
		m.backgroundCompactor, err = compaction.NewCompactor(docsPool,
			DocumentArrayPoolCapacity,
			opts.SegmentBuilderOptions(),
			opts.FSTSegmentOptions(),
			compaction.CompactorOptions{
				MmapDocsData: blockOpts.BackgroundCompactorMmapDocsData,
			})
		if err != nil {
			return err
		}
	}

	return nil
}

func taskNumBuilders(task compaction.Task) int {
	builders := 0
	for _, seg := range task.Segments {
		if seg.Builder != nil {
			builders++
			continue
		}
	}
	return builders
}
