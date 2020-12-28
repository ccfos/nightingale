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

// The wired list is the primary data structure that is used to support the LRU
// caching policy. It is a global (per-database) structure that is shared
// between all namespaces, shards, and series. It is responsible for determining
// which blocks should be kept "wired" (cached) in memory, and which should be
// closed and fetched again from disk if they need to be retrieved in the future.
//
// The WiredList is basically a specialized LRU, except that it doesn't store the
// data itself, it just keeps track of which data is currently in memory and makes
// decisions about which data to remove from memory. Updating the Wired List is
// asynchronous: callers put an operation to modify the list into a channel and
// a background goroutine pulls from that channels and performs updates to the
// list which may include removing items from memory ("unwiring" blocks).
//
// The WiredList itself does not allocate a per-entry datastructure to keep track
// of what is active and what is not. Instead, it creates a "virtual list" ontop
// of the existing blocks that are in memory by manipulating struct-level pointers
// on the DatabaseBlocks which are "owned" by the list. In other words, the
// DatabaseBlocks are scattered among numerous namespaces/shards/series, but they
// existed in virtual sorted order via the prev/next pointers they contain, but
// which are only manipulated by the WiredList.
//
// The WiredList ONLY keeps track of blocks that are read from disk. Blocks that
// are created by rotating recently-written data out of buffers and into new
// DatabaseBlocks are managed by the background ticks of the series. The background
// tick will avoid closing blocks that were read from disk, and a block will never
// be provided to the WiredList if it wasn't read from disk. This prevents tricky
// ownership semantics where both the background tick and and the WiredList are
// competing for ownership / trying to close the same blocks.

package block

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/m3db/m3/src/dbnode/clock"
	"github.com/m3db/m3/src/dbnode/runtime"
	"github.com/m3db/m3/src/x/instrument"

	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

const (
	defaultWiredListEventsChannelSize = 65536
	wiredListSampleGaugesEvery        = 100
)

var (
	errAlreadyStarted = errors.New("wired list already started")
	errAlreadyStopped = errors.New("wired list already stopped")
)

// WiredList is a database block wired list.
type WiredList struct {
	mu sync.RWMutex

	nowFn clock.NowFn

	// Max wired blocks, must use atomic store and load to access.
	maxWired int64

	root          dbBlock
	length        int
	updatesChSize int
	updatesCh     chan DatabaseBlock
	doneCh        chan struct{}

	metrics wiredListMetrics
	iOpts   instrument.Options
}

type wiredListMetrics struct {
	unwireable           tally.Gauge
	limit                tally.Gauge
	evicted              tally.Counter
	pushedBack           tally.Counter
	inserted             tally.Counter
	evictedAfterDuration tally.Timer
}

func newWiredListMetrics(scope tally.Scope) wiredListMetrics {
	return wiredListMetrics{
		// Keeps track of how many blocks are in the list
		unwireable: scope.Gauge("unwireable"),
		limit:      scope.Gauge("limit"),
		// Incremented when a block is evicted
		evicted: scope.Counter("evicted"),
		// Incremented when a block is "pushed back" in the list, I.E
		// it was already in the list
		pushedBack: scope.Counter("pushed-back"),
		// Incremented when a block is inserted into the list, I.E
		// it wasn't already present
		inserted: scope.Counter("inserted"),
		// Measure how much time blocks spend in the list before being evicted
		evictedAfterDuration: scope.Timer("evicted-after-duration"),
	}
}

// WiredListOptions is the options struct for the WiredList constructor.
type WiredListOptions struct {
	RuntimeOptionsManager runtime.OptionsManager
	InstrumentOptions     instrument.Options
	ClockOptions          clock.Options
	EventsChannelSize     int
}

// NewWiredList returns a new database block wired list.
func NewWiredList(opts WiredListOptions) *WiredList {
	scope := opts.InstrumentOptions.MetricsScope().
		SubScope("wired-list")
	l := &WiredList{
		nowFn:   opts.ClockOptions.NowFn(),
		metrics: newWiredListMetrics(scope),
		iOpts:   opts.InstrumentOptions,
	}
	if opts.EventsChannelSize > 0 {
		l.updatesChSize = opts.EventsChannelSize
	} else {
		l.updatesChSize = defaultWiredListEventsChannelSize
	}
	l.root.setNext(&l.root)
	l.root.setPrev(&l.root)
	opts.RuntimeOptionsManager.RegisterListener(l)
	return l
}

// SetRuntimeOptions sets the current runtime options to
// be consumed by the wired list
func (l *WiredList) SetRuntimeOptions(value runtime.Options) {
	atomic.StoreInt64(&l.maxWired, int64(value.MaxWiredBlocks()))
}

// Start starts processing the wired list
func (l *WiredList) Start() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.updatesCh != nil {
		return errAlreadyStarted
	}

	l.updatesCh = make(chan DatabaseBlock, l.updatesChSize)
	l.doneCh = make(chan struct{}, 1)
	go func() {
		i := 0
		for v := range l.updatesCh {
			l.processUpdateBlock(v)
			if i%wiredListSampleGaugesEvery == 0 {
				l.metrics.unwireable.Update(float64(l.length))
				l.metrics.limit.Update(float64(atomic.LoadInt64(&l.maxWired)))
			}
			i++
		}
		l.doneCh <- struct{}{}
	}()

	return nil
}

// Stop stops processing the wired list
func (l *WiredList) Stop() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.updatesCh == nil {
		return errAlreadyStopped
	}

	close(l.updatesCh)
	<-l.doneCh

	l.updatesCh = nil
	close(l.doneCh)
	l.doneCh = nil

	return nil
}

// BlockingUpdate places the block into the channel of blocks which are waiting to notify the
// wired list that they were accessed. All updates must be processed through this channel
// to force synchronization.
//
// We use a channel and a background processing goroutine to reduce blocking / lock contention.
func (l *WiredList) BlockingUpdate(v DatabaseBlock) {
	// Fast path, don't use defer (in Go 1.14 this won't matter anymore since
	// defer is basically compile time for simple callsites).
	l.mu.RLock()
	if l.updatesCh == nil {
		l.mu.RUnlock()
		return
	}
	l.updatesCh <- v
	l.mu.RUnlock()
}

// NonBlockingUpdate will attempt to put the block in the events channel, but will not block
// if the channel is full. Used in cases where a blocking update could trigger deadlock with
// the WiredList itself.
func (l *WiredList) NonBlockingUpdate(v DatabaseBlock) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.updatesCh == nil {
		return false
	}

	select {
	case l.updatesCh <- v:
		return true
	default:
		return false
	}
}

// processUpdateBlock inspects a block that has been modified or read recently
// and determines what outcome its state should have on the wired list.
func (l *WiredList) processUpdateBlock(v DatabaseBlock) {
	entry := v.wiredListEntry()

	// In some cases the WiredList can receive blocks that are closed. This can happen if a block is
	// in the updatesCh (because it was read) but also already in the WiredList, and while its still
	// in the updatesCh, it is evicted from the wired list to make room for some other block that is
	// being processed. The eviction of the block will close it, but the enqueued update is still in
	// the updateCh even though its an update for a closed block. For the same reason, the wired list
	// can receive blocks that were not retrieved from disk because the closed block was returned to
	// a pool and then re-used.
	unwireable := !entry.closed && entry.wasRetrievedFromDisk

	// If a block is still unwireable then its worth keeping track of in the wired list
	// so we push it back.
	if unwireable {
		l.pushBack(v)
		return
	}

	// If a block is not unwireable there is no point in keeping track of it in the WiredList,
	// so we remove it or don't add it in the first place. This works because the remove method
	// is a noop for blocks that aren't already in the WiredList and the pushBack method used
	// above is the only way for blocks to be added.
	l.remove(v)
}

func (l *WiredList) insertAfter(v, at DatabaseBlock) {
	now := l.nowFn()

	n := at.next()
	at.setNext(v)
	v.setPrev(at)
	v.setNext(n)
	n.setPrev(v)
	l.length++

	maxWired := int(atomic.LoadInt64(&l.maxWired))
	if maxWired <= 0 {
		// Not enforcing max wired blocks
		return
	}

	// Try to unwire all blocks possible
	bl := l.root.next()
	for l.length > maxWired && bl != &l.root {
		entry := bl.wiredListEntry()
		if !entry.wasRetrievedFromDisk {
			// This should never happen because processUpdateBlock performs the same
			// check, and a block should never be pooled in-between those steps because
			// the wired list is supposed to have sole ownership over that lifecycle and
			// is single-threaded.
			instrument.EmitAndLogInvariantViolation(l.iOpts, func(l *zap.Logger) {
				l.With(
					zap.Time("blockStart", entry.startTime),
					zap.Bool("closed", entry.closed),
					zap.Bool("wasRetrievedFromDisk", entry.wasRetrievedFromDisk),
				).Error("wired list tried to process a block that was not retrieved from disk")
			})

		}

		// Evict the block before closing it so that callers of series.ReadEncoded()
		// don't get errors about trying to read from a closed block.
		if onEvict := bl.OnEvictedFromWiredList(); onEvict != nil {
			if entry.seriesID == nil {
				// Entry should always have a series ID attached
				instrument.EmitAndLogInvariantViolation(l.iOpts, func(l *zap.Logger) {
					l.With(
						zap.Time("blockStart", entry.startTime),
						zap.Bool("closed", entry.closed),
						zap.Bool("wasRetrievedFromDisk", entry.wasRetrievedFromDisk),
					).Error("wired list entry does not have seriesID set")
				})

			} else {
				onEvict.OnEvictedFromWiredList(entry.seriesID, entry.startTime)
			}
		}

		// bl.CloseIfFromDisk() will return the block to the pool. In order to avoid
		// races with the pool itself, we capture the value of the next block and
		// remove the block from the wired list before we close it.
		nextBl := bl.next()
		l.remove(bl)
		if wasFromDisk := bl.CloseIfFromDisk(); !wasFromDisk {
			// Should never happen
			instrument.EmitAndLogInvariantViolation(l.iOpts, func(l *zap.Logger) {
				l.With(
					zap.Time("blockStart", entry.startTime),
					zap.Bool("closed", entry.closed),
					zap.Bool("wasRetrievedFromDisk", entry.wasRetrievedFromDisk),
				).Error("wired list tried to close a block that was not from disk")
			})
		}

		l.metrics.evicted.Inc(1)

		enteredListAt := time.Unix(0, bl.enteredListAtUnixNano())
		l.metrics.evictedAfterDuration.Record(now.Sub(enteredListAt))

		bl = nextBl
	}
}

func (l *WiredList) remove(v DatabaseBlock) {
	if !l.exists(v) {
		// Already removed
		return
	}
	v.prev().setNext(v.next())
	v.next().setPrev(v.prev())
	v.setNext(nil) // avoid memory leaks
	v.setPrev(nil) // avoid memory leaks
	l.length--
}

func (l *WiredList) pushBack(v DatabaseBlock) {
	if l.exists(v) {
		l.metrics.pushedBack.Inc(1)
		l.moveToBack(v)
		return
	}

	l.metrics.inserted.Inc(1)
	l.insertAfter(v, l.root.prev())
	v.setEnteredListAtUnixNano(l.nowFn().UnixNano())
}

func (l *WiredList) moveToBack(v DatabaseBlock) {
	if !l.exists(v) || l.root.prev() == v {
		return
	}
	l.remove(v)
	l.insertAfter(v, l.root.prev())
}

func (l *WiredList) exists(v DatabaseBlock) bool {
	return v.next() != nil || v.prev() != nil
}
