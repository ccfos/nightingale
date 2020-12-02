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

package client

import (
	"fmt"
	"time"

	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/namespace"
	"github.com/m3db/m3/src/dbnode/storage/block"
	"github.com/m3db/m3/src/dbnode/storage/bootstrap/result"
	"github.com/m3db/m3/src/dbnode/storage/index"
	"github.com/m3db/m3/src/dbnode/topology"
	"github.com/m3db/m3/src/x/ident"
	m3sync "github.com/m3db/m3/src/x/sync"
	xtime "github.com/m3db/m3/src/x/time"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

type newSessionFn func(Options) (clientSession, error)

// replicatedSession is an implementation of clientSession which replicates
// session read/writes to a set of clusters asynchronously.
type replicatedSession struct {
	session              clientSession
	asyncSessions        []clientSession
	newSessionFn         newSessionFn
	workerPool           m3sync.PooledWorkerPool
	replicationSemaphore chan struct{}
	scope                tally.Scope
	log                  *zap.Logger
	metrics              replicatedSessionMetrics
	outCh                chan error
	writeTimestampOffset time.Duration
}

type replicatedSessionMetrics struct {
	replicateExecuted    tally.Counter
	replicateNotExecuted tally.Counter
	replicateError       tally.Counter
	replicateSuccess     tally.Counter
}

func newReplicatedSessionMetrics(scope tally.Scope) replicatedSessionMetrics {
	return replicatedSessionMetrics{
		replicateExecuted:    scope.Counter("replicate.executed"),
		replicateNotExecuted: scope.Counter("replicate.not-executed"),
		replicateError:       scope.Counter("replicate.error"),
		replicateSuccess:     scope.Counter("replicate.success"),
	}
}

// Ensure replicatedSession implements the clientSession interface.
var _ clientSession = (*replicatedSession)(nil)

type replicatedSessionOption func(*replicatedSession)

func withNewSessionFn(fn newSessionFn) replicatedSessionOption {
	return func(session *replicatedSession) {
		session.newSessionFn = fn
	}
}

func newReplicatedSession(opts Options, asyncOpts []Options, options ...replicatedSessionOption) (clientSession, error) {
	workerPool := opts.AsyncWriteWorkerPool()

	scope := opts.InstrumentOptions().MetricsScope()

	session := replicatedSession{
		newSessionFn:         newSession,
		workerPool:           workerPool,
		replicationSemaphore: make(chan struct{}, opts.AsyncWriteMaxConcurrency()),
		scope:                scope,
		log:                  opts.InstrumentOptions().Logger(),
		metrics:              newReplicatedSessionMetrics(scope),
		writeTimestampOffset: opts.WriteTimestampOffset(),
	}

	// Apply options
	for _, option := range options {
		option(&session)
	}

	if err := session.setSession(opts); err != nil {
		return nil, err
	}
	if err := session.setAsyncSessions(asyncOpts); err != nil {
		return nil, err
	}

	return &session, nil
}

type writeFunc func(Session) error

func (s *replicatedSession) setSession(opts Options) error {
	if opts.TopologyInitializer() == nil {
		return nil
	}

	session, err := s.newSessionFn(opts)
	if err != nil {
		return err
	}
	s.session = session
	return nil
}

func (s *replicatedSession) setAsyncSessions(opts []Options) error {
	sessions := make([]clientSession, 0, len(opts))
	for i, oo := range opts {
		subscope := oo.InstrumentOptions().MetricsScope().SubScope(fmt.Sprintf("async-%d", i))
		oo = oo.SetInstrumentOptions(oo.InstrumentOptions().SetMetricsScope(subscope))

		session, err := s.newSessionFn(oo)
		if err != nil {
			return err
		}
		sessions = append(sessions, session)
	}
	s.asyncSessions = sessions
	return nil
}

type replicatedParams struct {
	namespace  ident.ID
	id         ident.ID
	t          time.Time
	value      float64
	unit       xtime.Unit
	annotation []byte
	tags       ident.TagIterator
	useTags    bool
}

// NB(srobb): it would be a nicer to accept a lambda which is the fn to
// be performed on all sessions, however this causes an extra allocation.
func (s replicatedSession) replicate(params replicatedParams) error {
	for _, asyncSession := range s.asyncSessions {
		asyncSession := asyncSession // capture var
		select {
		case s.replicationSemaphore <- struct{}{}:
			s.workerPool.Go(func() {
				var err error
				if params.useTags {
					err = asyncSession.WriteTagged(params.namespace, params.id, params.tags, params.t, params.value, params.unit, params.annotation)
				} else {
					err = asyncSession.Write(params.namespace, params.id, params.t, params.value, params.unit, params.annotation)
				}
				if err != nil {
					s.metrics.replicateError.Inc(1)
					s.log.Error("could not replicate write", zap.Error(err))
				} else {
					s.metrics.replicateSuccess.Inc(1)
				}
				if s.outCh != nil {
					s.outCh <- err
				}
				<-s.replicationSemaphore
			})
			s.metrics.replicateExecuted.Inc(1)
		default:
			s.metrics.replicateNotExecuted.Inc(1)
		}
	}

	if params.useTags {
		return s.session.WriteTagged(params.namespace, params.id, params.tags, params.t, params.value, params.unit, params.annotation)
	}
	return s.session.Write(params.namespace, params.id, params.t, params.value, params.unit, params.annotation)
}

// Write value to the database for an ID.
func (s replicatedSession) Write(namespace, id ident.ID, t time.Time, value float64, unit xtime.Unit, annotation []byte) error {
	return s.replicate(replicatedParams{
		namespace:  namespace,
		id:         id,
		t:          t.Add(-s.writeTimestampOffset),
		value:      value,
		unit:       unit,
		annotation: annotation,
	})
}

// WriteTagged value to the database for an ID and given tags.
func (s replicatedSession) WriteTagged(namespace, id ident.ID, tags ident.TagIterator, t time.Time, value float64, unit xtime.Unit, annotation []byte) error {
	return s.replicate(replicatedParams{
		namespace:  namespace,
		id:         id,
		t:          t.Add(-s.writeTimestampOffset),
		value:      value,
		unit:       unit,
		annotation: annotation,
		tags:       tags,
		useTags:    true,
	})
}

// Fetch values from the database for an ID.
func (s replicatedSession) Fetch(namespace, id ident.ID, startInclusive, endExclusive time.Time) (encoding.SeriesIterator, error) {
	return s.session.Fetch(namespace, id, startInclusive, endExclusive)
}

// FetchIDs values from the database for a set of IDs.
func (s replicatedSession) FetchIDs(namespace ident.ID, ids ident.Iterator, startInclusive, endExclusive time.Time) (encoding.SeriesIterators, error) {
	return s.session.FetchIDs(namespace, ids, startInclusive, endExclusive)
}

// Aggregate aggregates values from the database for the given set of constraints.
func (s replicatedSession) Aggregate(
	ns ident.ID, q index.Query, opts index.AggregationOptions,
) (AggregatedTagsIterator, FetchResponseMetadata, error) {
	return s.session.Aggregate(ns, q, opts)
}

// FetchTagged resolves the provided query to known IDs, and fetches the data for them.
func (s replicatedSession) FetchTagged(namespace ident.ID, q index.Query, opts index.QueryOptions) (encoding.SeriesIterators, FetchResponseMetadata, error) {
	return s.session.FetchTagged(namespace, q, opts)
}

// FetchTaggedIDs resolves the provided query to known IDs.
func (s replicatedSession) FetchTaggedIDs(namespace ident.ID, q index.Query, opts index.QueryOptions) (TaggedIDsIterator, FetchResponseMetadata, error) {
	return s.session.FetchTaggedIDs(namespace, q, opts)
}

// ShardID returns the given shard for an ID for callers
// to easily discern what shard is failing when operations
// for given IDs begin failing.
func (s replicatedSession) ShardID(id ident.ID) (uint32, error) {
	return s.session.ShardID(id)
}

// IteratorPools exposes the internal iterator pools used by the session to clients.
func (s replicatedSession) IteratorPools() (encoding.IteratorPools, error) {
	return s.session.IteratorPools()
}

// Close the session.
func (s replicatedSession) Close() error {
	err := s.session.Close()
	for _, as := range s.asyncSessions {
		if err := as.Close(); err != nil {
			s.log.Error("could not close async session: %v", zap.Error(err))
		}
	}
	return err
}

// Origin returns the host that initiated the session.
func (s replicatedSession) Origin() topology.Host {
	return s.session.Origin()
}

// Replicas returns the replication factor.
func (s replicatedSession) Replicas() int {
	return s.session.Replicas()
}

// TopologyMap returns the current topology map. Note that the session
// has a separate topology watch than the database itself, so the two
// values can be out of sync and this method should not be relied upon
// if the current view of the topology as seen by the database is required.
func (s replicatedSession) TopologyMap() (topology.Map, error) {
	return s.session.TopologyMap()
}

// Truncate will truncate the namespace for a given shard.
func (s replicatedSession) Truncate(namespace ident.ID) (int64, error) {
	return s.session.Truncate(namespace)
}

// FetchBootstrapBlocksFromPeers will fetch the most fulfilled block
// for each series using the runtime configurable bootstrap level consistency.
func (s replicatedSession) FetchBootstrapBlocksFromPeers(
	namespace namespace.Metadata,
	shard uint32,
	start, end time.Time,
	opts result.Options,
) (result.ShardResult, error) {
	return s.session.FetchBootstrapBlocksFromPeers(namespace, shard, start, end, opts)
}

// FetchBootstrapBlocksMetadataFromPeers will fetch the blocks metadata from
// available peers using the runtime configurable bootstrap level consistency.
func (s replicatedSession) FetchBootstrapBlocksMetadataFromPeers(
	namespace ident.ID,
	shard uint32,
	start, end time.Time,
	result result.Options,
) (PeerBlockMetadataIter, error) {
	return s.session.FetchBootstrapBlocksMetadataFromPeers(namespace, shard, start, end, result)
}

// FetchBlocksMetadataFromPeers will fetch the blocks metadata from
// available peers.
func (s replicatedSession) FetchBlocksMetadataFromPeers(
	namespace ident.ID,
	shard uint32,
	start, end time.Time,
	consistencyLevel topology.ReadConsistencyLevel,
	result result.Options,
) (PeerBlockMetadataIter, error) {
	return s.session.FetchBlocksMetadataFromPeers(namespace, shard, start, end, consistencyLevel, result)
}

// FetchBlocksFromPeers will fetch the required blocks from the
// peers specified.
func (s replicatedSession) FetchBlocksFromPeers(
	namespace namespace.Metadata,
	shard uint32,
	consistencyLevel topology.ReadConsistencyLevel,
	metadatas []block.ReplicaMetadata,
	opts result.Options,
) (PeerBlocksIter, error) {
	return s.session.FetchBlocksFromPeers(namespace, shard, consistencyLevel, metadatas, opts)
}

// Open the client session.
func (s replicatedSession) Open() error {
	if err := s.session.Open(); err != nil {
		return err
	}
	for _, asyncSession := range s.asyncSessions {
		if err := asyncSession.Open(); err != nil {
			s.log.Error("could not open session to async cluster: %v", zap.Error(err))
		}
	}
	return nil
}
