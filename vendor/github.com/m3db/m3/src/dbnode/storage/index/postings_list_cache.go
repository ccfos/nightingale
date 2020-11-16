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

package index

import (
	"sync"
	"time"

	"github.com/m3db/m3/src/m3ninx/postings"
	"github.com/m3db/m3/src/x/instrument"

	"github.com/pborman/uuid"
	"github.com/uber-go/tally"
)

// PatternType is an enum for the various pattern types. It allows us
// separate them logically within the cache.
type PatternType int

// Closer represents a function that will close managed resources.
type Closer func()

const (
	// PatternTypeRegexp indicates that the pattern is of type regexp.
	PatternTypeRegexp PatternType = iota
	// PatternTypeTerm indicates that the pattern is of type term.
	PatternTypeTerm
	// PatternTypeField indicates that the pattern is of type field.
	PatternTypeField

	reportLoopInterval = 10 * time.Second
	emptyPattern       = ""
)

// PostingsListCacheOptions is the options struct for the query cache.
type PostingsListCacheOptions struct {
	InstrumentOptions instrument.Options
}

// PostingsListCache implements an LRU for caching queries and their results.
type PostingsListCache struct {
	sync.Mutex

	lru *postingsListLRU

	size    int
	opts    PostingsListCacheOptions
	metrics *postingsListCacheMetrics
}

// NewPostingsListCache creates a new query cache.
func NewPostingsListCache(size int, opts PostingsListCacheOptions) (*PostingsListCache, Closer, error) {
	lru, err := newPostingsListLRU(size)
	if err != nil {
		return nil, nil, err
	}

	plc := &PostingsListCache{
		lru:     lru,
		size:    size,
		opts:    opts,
		metrics: newPostingsListCacheMetrics(opts.InstrumentOptions.MetricsScope()),
	}

	closer := plc.startReportLoop()
	return plc, closer, nil
}

// GetRegexp returns the cached results for the provided regexp query, if any.
func (q *PostingsListCache) GetRegexp(
	segmentUUID uuid.UUID,
	field string,
	pattern string,
) (postings.List, bool) {
	return q.get(segmentUUID, field, pattern, PatternTypeRegexp)
}

// GetTerm returns the cached results for the provided term query, if any.
func (q *PostingsListCache) GetTerm(
	segmentUUID uuid.UUID,
	field string,
	pattern string,
) (postings.List, bool) {
	return q.get(segmentUUID, field, pattern, PatternTypeTerm)
}

// GetField returns the cached results for the provided field query, if any.
func (q *PostingsListCache) GetField(
	segmentUUID uuid.UUID,
	field string,
) (postings.List, bool) {
	return q.get(segmentUUID, field, emptyPattern, PatternTypeField)
}

func (q *PostingsListCache) get(
	segmentUUID uuid.UUID,
	field string,
	pattern string,
	patternType PatternType,
) (postings.List, bool) {
	// No RLock because a Get() operation mutates the LRU.
	q.Lock()
	p, ok := q.lru.Get(segmentUUID, field, pattern, patternType)
	q.Unlock()

	q.emitCacheGetMetrics(patternType, ok)

	if !ok {
		return nil, false
	}

	return p, ok
}

// PutRegexp updates the LRU with the result of the regexp query.
func (q *PostingsListCache) PutRegexp(
	segmentUUID uuid.UUID,
	field string,
	pattern string,
	pl postings.List,
) {
	q.put(segmentUUID, field, pattern, PatternTypeRegexp, pl)
}

// PutTerm updates the LRU with the result of the term query.
func (q *PostingsListCache) PutTerm(
	segmentUUID uuid.UUID,
	field string,
	pattern string,
	pl postings.List,
) {
	q.put(segmentUUID, field, pattern, PatternTypeTerm, pl)
}

// PutField updates the LRU with the result of the field query.
func (q *PostingsListCache) PutField(
	segmentUUID uuid.UUID,
	field string,
	pl postings.List,
) {
	q.put(segmentUUID, field, emptyPattern, PatternTypeField, pl)
}

func (q *PostingsListCache) put(
	segmentUUID uuid.UUID,
	field string,
	pattern string,
	patternType PatternType,
	pl postings.List,
) {
	q.Lock()
	q.lru.Add(
		segmentUUID,
		field,
		pattern,
		patternType,
		pl,
	)
	q.Unlock()
	q.emitCachePutMetrics(patternType)
}

// PurgeSegment removes all postings lists associated with the specified
// segment from the cache.
func (q *PostingsListCache) PurgeSegment(segmentUUID uuid.UUID) {
	q.Lock()
	q.lru.PurgeSegment(segmentUUID)
	q.Unlock()
}

// startReportLoop starts a background process that will call Report()
// on a regular basis and returns a function that will end the background
// process.
func (q *PostingsListCache) startReportLoop() Closer {
	doneCh := make(chan struct{})

	go func() {
		for {
			select {
			case <-doneCh:
				return
			default:
			}

			q.Report()
			time.Sleep(reportLoopInterval)
		}
	}()

	return func() { close(doneCh) }
}

// Report will emit metrics about the status of the cache.
func (q *PostingsListCache) Report() {
	var (
		size     float64
		capacity float64
	)

	q.Lock()
	size = float64(q.lru.Len())
	capacity = float64(q.size)
	q.Unlock()

	q.metrics.size.Update(size)
	q.metrics.capacity.Update(capacity)
}

func (q *PostingsListCache) emitCacheGetMetrics(patternType PatternType, hit bool) {
	var method *postingsListCacheMethodMetrics
	switch patternType {
	case PatternTypeRegexp:
		method = q.metrics.regexp
	case PatternTypeTerm:
		method = q.metrics.term
	case PatternTypeField:
		method = q.metrics.field
	default:
		method = q.metrics.unknown // should never happen
	}
	if hit {
		method.hits.Inc(1)
	} else {
		method.misses.Inc(1)
	}
}

func (q *PostingsListCache) emitCachePutMetrics(patternType PatternType) {
	switch patternType {
	case PatternTypeRegexp:
		q.metrics.regexp.puts.Inc(1)
	case PatternTypeTerm:
		q.metrics.term.puts.Inc(1)
	case PatternTypeField:
		q.metrics.field.puts.Inc(1)
	default:
		q.metrics.unknown.puts.Inc(1) // should never happen
	}
}

type postingsListCacheMetrics struct {
	regexp  *postingsListCacheMethodMetrics
	term    *postingsListCacheMethodMetrics
	field   *postingsListCacheMethodMetrics
	unknown *postingsListCacheMethodMetrics

	size     tally.Gauge
	capacity tally.Gauge
}

func newPostingsListCacheMetrics(scope tally.Scope) *postingsListCacheMetrics {
	return &postingsListCacheMetrics{
		regexp: newPostingsListCacheMethodMetrics(scope.Tagged(map[string]string{
			"query_type": "regexp",
		})),
		term: newPostingsListCacheMethodMetrics(scope.Tagged(map[string]string{
			"query_type": "term",
		})),
		field: newPostingsListCacheMethodMetrics(scope.Tagged(map[string]string{
			"query_type": "field",
		})),
		unknown: newPostingsListCacheMethodMetrics(scope.Tagged(map[string]string{
			"query_type": "unknown",
		})),

		size:     scope.Gauge("size"),
		capacity: scope.Gauge("capacity"),
	}
}

type postingsListCacheMethodMetrics struct {
	hits   tally.Counter
	misses tally.Counter
	puts   tally.Counter
}

func newPostingsListCacheMethodMetrics(scope tally.Scope) *postingsListCacheMethodMetrics {
	return &postingsListCacheMethodMetrics{
		hits:   scope.Counter("hits"),
		misses: scope.Counter("misses"),
		puts:   scope.Counter("puts"),
	}
}
