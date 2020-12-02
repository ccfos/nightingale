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

package client

import (
	"time"

	"github.com/m3db/m3/src/dbnode/clock"
	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/generated/thrift/rpc"
	"github.com/m3db/m3/src/dbnode/namespace"
	"github.com/m3db/m3/src/dbnode/runtime"
	"github.com/m3db/m3/src/dbnode/storage/block"
	"github.com/m3db/m3/src/dbnode/storage/bootstrap/result"
	"github.com/m3db/m3/src/dbnode/storage/index"
	"github.com/m3db/m3/src/dbnode/topology"
	"github.com/m3db/m3/src/x/context"
	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/m3/src/x/instrument"
	"github.com/m3db/m3/src/x/pool"
	xretry "github.com/m3db/m3/src/x/retry"
	"github.com/m3db/m3/src/x/sampler"
	"github.com/m3db/m3/src/x/serialize"
	xsync "github.com/m3db/m3/src/x/sync"
	xtime "github.com/m3db/m3/src/x/time"

	tchannel "github.com/uber/tchannel-go"
)

// Client can create sessions to write and read to a cluster.
type Client interface {
	// Options returns the Client Options.
	Options() Options

	// NewSession creates a new session.
	NewSession() (Session, error)

	// DefaultSession creates a default session that gets reused.
	DefaultSession() (Session, error)

	// DefaultSessionActive returns whether the default session is active.
	DefaultSessionActive() bool
}

// Session can write and read to a cluster.
type Session interface {
	// Write value to the database for an ID.
	Write(namespace, id ident.ID, t time.Time, value float64, unit xtime.Unit, annotation []byte) error

	// WriteTagged value to the database for an ID and given tags.
	WriteTagged(namespace, id ident.ID, tags ident.TagIterator, t time.Time, value float64, unit xtime.Unit, annotation []byte) error

	// Fetch values from the database for an ID.
	Fetch(namespace, id ident.ID, startInclusive, endExclusive time.Time) (encoding.SeriesIterator, error)

	// FetchIDs values from the database for a set of IDs.
	FetchIDs(namespace ident.ID, ids ident.Iterator, startInclusive, endExclusive time.Time) (encoding.SeriesIterators, error)

	// FetchTagged resolves the provided query to known IDs, and fetches the data for them.
	FetchTagged(namespace ident.ID, q index.Query, opts index.QueryOptions) (encoding.SeriesIterators, FetchResponseMetadata, error)

	// FetchTaggedIDs resolves the provided query to known IDs.
	FetchTaggedIDs(namespace ident.ID, q index.Query, opts index.QueryOptions) (TaggedIDsIterator, FetchResponseMetadata, error)

	// Aggregate aggregates values from the database for the given set of constraints.
	Aggregate(namespace ident.ID, q index.Query, opts index.AggregationOptions) (AggregatedTagsIterator, FetchResponseMetadata, error)

	// ShardID returns the given shard for an ID for callers
	// to easily discern what shard is failing when operations
	// for given IDs begin failing.
	ShardID(id ident.ID) (uint32, error)

	// IteratorPools exposes the internal iterator pools used by the session to clients.
	IteratorPools() (encoding.IteratorPools, error)

	// Close the session
	Close() error
}

// FetchResponseMetadata is metadata about a fetch response.
type FetchResponseMetadata struct {
	// Exhaustive indicates whether the underlying data set presents a full
	// collection of retrieved data.
	Exhaustive bool
	// Responses is the count of responses.
	Responses int
	// EstimateTotalBytes is an approximation of the total byte size of the response.
	EstimateTotalBytes int
}

// AggregatedTagsIterator iterates over a collection of tag names with optionally
// associated values.
type AggregatedTagsIterator interface {
	// Next returns whether there are more items in the collection.
	Next() bool

	// Remaining returns the number of elements remaining to be iterated over.
	Remaining() int

	// Current returns the current tagName, and associated tagValues iterator.
	// These remain valid until Next() is called again.
	Current() (tagName ident.ID, tagValues ident.Iterator)

	// Err returns any error encountered.
	Err() error

	// Finalize releases any held resources.
	Finalize()
}

// TaggedIDsIterator iterates over a collection of IDs with associated tags and namespace.
type TaggedIDsIterator interface {
	// Next returns whether there are more items in the collection.
	Next() bool

	// Remaining returns the number of elements remaining to be iterated over.
	Remaining() int

	// Current returns the ID, Tags and Namespace for a single timeseries.
	// These remain valid until Next() is called again.
	Current() (namespaceID ident.ID, seriesID ident.ID, tags ident.TagIterator)

	// Err returns any error encountered.
	Err() error

	// Finalize releases any held resources.
	Finalize()
}

// AdminClient can create administration sessions.
type AdminClient interface {
	Client

	// NewSession creates a new session.
	NewAdminSession() (AdminSession, error)

	// DefaultAdminSession creates a default admin session that gets reused.
	DefaultAdminSession() (AdminSession, error)
}

// PeerBlockMetadataIter iterates over a collection of
// blocks metadata from peers.
type PeerBlockMetadataIter interface {
	// Next returns whether there are more items in the collection.
	Next() bool

	// Current returns the host and block metadata, which remain
	// valid until Next() is called again.
	Current() (topology.Host, block.Metadata)

	// Err returns any error encountered
	Err() error
}

// PeerBlocksIter iterates over a collection of blocks from peers.
type PeerBlocksIter interface {
	// Next returns whether there are more items in the collection.
	Next() bool

	// Current returns the metadata, and block data for a single block replica.
	// These remain valid until Next() is called again.
	Current() (topology.Host, ident.ID, block.DatabaseBlock)

	// Err returns any error encountered.
	Err() error
}

// AdminSession can perform administrative and node-to-node operations.
type AdminSession interface {
	Session

	// Origin returns the host that initiated the session.
	Origin() topology.Host

	// Replicas returns the replication factor.
	Replicas() int

	// TopologyMap returns the current topology map. Note that the session
	// has a separate topology watch than the database itself, so the two
	// values can be out of sync and this method should not be relied upon
	// if the current view of the topology as seen by the database is required.
	TopologyMap() (topology.Map, error)

	// Truncate will truncate the namespace for a given shard.
	Truncate(namespace ident.ID) (int64, error)

	// FetchBootstrapBlocksFromPeers will fetch the most fulfilled block
	// for each series using the runtime configurable bootstrap level consistency.
	FetchBootstrapBlocksFromPeers(
		namespace namespace.Metadata,
		shard uint32,
		start, end time.Time,
		opts result.Options,
	) (result.ShardResult, error)

	// FetchBootstrapBlocksMetadataFromPeers will fetch the blocks metadata from
	// available peers using the runtime configurable bootstrap level consistency.
	FetchBootstrapBlocksMetadataFromPeers(
		namespace ident.ID,
		shard uint32,
		start, end time.Time,
		result result.Options,
	) (PeerBlockMetadataIter, error)

	// FetchBlocksMetadataFromPeers will fetch the blocks metadata from
	// available peers.
	FetchBlocksMetadataFromPeers(
		namespace ident.ID,
		shard uint32,
		start, end time.Time,
		consistencyLevel topology.ReadConsistencyLevel,
		result result.Options,
	) (PeerBlockMetadataIter, error)

	// FetchBlocksFromPeers will fetch the required blocks from the
	// peers specified.
	FetchBlocksFromPeers(
		namespace namespace.Metadata,
		shard uint32,
		consistencyLevel topology.ReadConsistencyLevel,
		metadatas []block.ReplicaMetadata,
		opts result.Options,
	) (PeerBlocksIter, error)
}

// Options is a set of client options.
type Options interface {
	// Validate validates the options.
	Validate() error

	// SetEncodingM3TSZ sets M3TSZ encoding.
	SetEncodingM3TSZ() Options

	// SetEncodingProto sets proto encoding.
	SetEncodingProto(encodingOpts encoding.Options) Options

	// IsSetEncodingProto returns whether proto encoding is set.
	IsSetEncodingProto() bool

	// SetRuntimeOptionsManager sets the runtime options manager, it is optional
	SetRuntimeOptionsManager(value runtime.OptionsManager) Options

	// RuntimeOptionsManager returns the runtime options manager, it is optional.
	RuntimeOptionsManager() runtime.OptionsManager

	// SetClockOptions sets the clock options.
	SetClockOptions(value clock.Options) Options

	// ClockOptions returns the clock options.
	ClockOptions() clock.Options

	// SetInstrumentOptions sets the instrumentation options.
	SetInstrumentOptions(value instrument.Options) Options

	// InstrumentOptions returns the instrumentation options.
	InstrumentOptions() instrument.Options

	// SetLogErrorSampleRate sets the log error sample rate between [0,1.0].
	SetLogErrorSampleRate(value sampler.Rate) Options

	// LogErrorSampleRate returns the log error sample rate between [0,1.0].
	LogErrorSampleRate() sampler.Rate

	// SetTopologyInitializer sets the TopologyInitializer.
	SetTopologyInitializer(value topology.Initializer) Options

	// TopologyInitializer returns the TopologyInitializer.
	TopologyInitializer() topology.Initializer

	// SetReadConsistencyLevel sets the read consistency level.
	SetReadConsistencyLevel(value topology.ReadConsistencyLevel) Options

	// topology.ReadConsistencyLevel returns the read consistency level.
	ReadConsistencyLevel() topology.ReadConsistencyLevel

	// SetWriteConsistencyLevel sets the write consistency level.
	SetWriteConsistencyLevel(value topology.ConsistencyLevel) Options

	// WriteConsistencyLevel returns the write consistency level.
	WriteConsistencyLevel() topology.ConsistencyLevel

	// SetChannelOptions sets the channelOptions.
	SetChannelOptions(value *tchannel.ChannelOptions) Options

	// ChannelOptions returns the channelOptions.
	ChannelOptions() *tchannel.ChannelOptions

	// SetMaxConnectionCount sets the maxConnectionCount.
	SetMaxConnectionCount(value int) Options

	// MaxConnectionCount returns the maxConnectionCount.
	MaxConnectionCount() int

	// SetMinConnectionCount sets the minConnectionCount.
	SetMinConnectionCount(value int) Options

	// MinConnectionCount returns the minConnectionCount.
	MinConnectionCount() int

	// SetHostConnectTimeout sets the hostConnectTimeout.
	SetHostConnectTimeout(value time.Duration) Options

	// HostConnectTimeout returns the hostConnectTimeout.
	HostConnectTimeout() time.Duration

	// SetClusterConnectTimeout sets the clusterConnectTimeout.
	SetClusterConnectTimeout(value time.Duration) Options

	// ClusterConnectTimeout returns the clusterConnectTimeout.
	ClusterConnectTimeout() time.Duration

	// SetClusterConnectConsistencyLevel sets the clusterConnectConsistencyLevel.
	SetClusterConnectConsistencyLevel(value topology.ConnectConsistencyLevel) Options

	// ClusterConnectConsistencyLevel returns the clusterConnectConsistencyLevel.
	ClusterConnectConsistencyLevel() topology.ConnectConsistencyLevel

	// SetWriteRequestTimeout sets the writeRequestTimeout.
	SetWriteRequestTimeout(value time.Duration) Options

	// WriteRequestTimeout returns the writeRequestTimeout.
	WriteRequestTimeout() time.Duration

	// SetFetchRequestTimeout sets the fetchRequestTimeout.
	SetFetchRequestTimeout(value time.Duration) Options

	// FetchRequestTimeout returns the fetchRequestTimeout.
	FetchRequestTimeout() time.Duration

	// SetTruncateRequestTimeout sets the truncateRequestTimeout.
	SetTruncateRequestTimeout(value time.Duration) Options

	// TruncateRequestTimeout returns the truncateRequestTimeout.
	TruncateRequestTimeout() time.Duration

	// SetBackgroundConnectInterval sets the backgroundConnectInterval.
	SetBackgroundConnectInterval(value time.Duration) Options

	// BackgroundConnectInterval returns the backgroundConnectInterval.
	BackgroundConnectInterval() time.Duration

	// SetBackgroundConnectStutter sets the backgroundConnectStutter.
	SetBackgroundConnectStutter(value time.Duration) Options

	// BackgroundConnectStutter returns the backgroundConnectStutter.
	BackgroundConnectStutter() time.Duration

	// SetBackgroundHealthCheckInterval sets the background health check interval
	SetBackgroundHealthCheckInterval(value time.Duration) Options

	// BackgroundHealthCheckInterval returns the background health check interval
	BackgroundHealthCheckInterval() time.Duration

	// SetBackgroundHealthCheckStutter sets the background health check stutter
	SetBackgroundHealthCheckStutter(value time.Duration) Options

	// BackgroundHealthCheckStutter returns the background health check stutter
	BackgroundHealthCheckStutter() time.Duration

	// SetBackgroundHealthCheckFailLimit sets the background health failure
	// limit before connection is deemed unhealth
	SetBackgroundHealthCheckFailLimit(value int) Options

	// BackgroundHealthCheckFailLimit returns the background health failure
	// limit before connection is deemed unhealth
	BackgroundHealthCheckFailLimit() int

	// SetBackgroundHealthCheckFailThrottleFactor sets the throttle factor to
	// apply when calculating how long to wait between a failed health check and
	// a retry attempt. It is applied by multiplying against the host connect
	// timeout to produce a throttle sleep value.
	SetBackgroundHealthCheckFailThrottleFactor(value float64) Options

	// BackgroundHealthCheckFailThrottleFactor returns the throttle factor to
	// apply when calculating how long to wait between a failed health check and
	// a retry attempt. It is applied by multiplying against the host connect
	// timeout to produce a throttle sleep value.
	BackgroundHealthCheckFailThrottleFactor() float64

	// SetWriteRetrier sets the write retrier when performing a write for
	// a write operation. Only retryable errors are retried.
	SetWriteRetrier(value xretry.Retrier) Options

	// WriteRetrier returns the write retrier when perform a write for
	// a write operation. Only retryable errors are retried.
	WriteRetrier() xretry.Retrier

	// SetFetchRetrier sets the fetch retrier when performing a write for
	// a fetch operation. Only retryable errors are retried.
	SetFetchRetrier(value xretry.Retrier) Options

	// FetchRetrier returns the fetch retrier when performing a fetch for
	// a fetch operation. Only retryable errors are retried.
	FetchRetrier() xretry.Retrier

	// SetWriteShardsInitializing sets whether to write to shards that are
	// initializing or not.
	SetWriteShardsInitializing(value bool) Options

	// WriteShardsInitializing returns whether to write to shards that are
	// initializing or not.
	WriteShardsInitializing() bool

	// SetShardsLeavingCountTowardsConsistency sets whether to count shards
	// that are leaving or not towards consistency level calculations.
	SetShardsLeavingCountTowardsConsistency(value bool) Options

	// ShardsLeavingCountTowardsConsistency returns whether to count shards
	// that are leaving or not towards consistency level calculations.
	ShardsLeavingCountTowardsConsistency() bool

	// SetTagEncoderOptions sets the TagEncoderOptions.
	SetTagEncoderOptions(value serialize.TagEncoderOptions) Options

	// TagEncoderOptions returns the TagEncoderOptions.
	TagEncoderOptions() serialize.TagEncoderOptions

	// SetTagEncoderPoolSize sets the TagEncoderPoolSize.
	SetTagEncoderPoolSize(value int) Options

	// TagEncoderPoolSize returns the TagEncoderPoolSize.
	TagEncoderPoolSize() int

	// SetTagDecoderOptions sets the TagDecoderOptions.
	SetTagDecoderOptions(value serialize.TagDecoderOptions) Options

	// TagDecoderOptions returns the TagDecoderOptions.
	TagDecoderOptions() serialize.TagDecoderOptions

	// SetTagDecoderPoolSize sets the TagDecoderPoolSize.
	SetTagDecoderPoolSize(value int) Options

	// TagDecoderPoolSize returns the TagDecoderPoolSize.
	TagDecoderPoolSize() int

	// SetWriteBatchSize sets the writeBatchSize
	// NB(r): for a write only application load this should match the host
	// queue ops flush size so that each time a host queue is flushed it can
	// fit the entire flushed write ops into a single batch.
	SetWriteBatchSize(value int) Options

	// WriteBatchSize returns the writeBatchSize.
	WriteBatchSize() int

	// SetFetchBatchSize sets the fetchBatchSize
	// NB(r): for a fetch only application load this should match the host
	// queue ops flush size so that each time a host queue is flushed it can
	// fit the entire flushed fetch ops into a single batch.
	SetFetchBatchSize(value int) Options

	// FetchBatchSize returns the fetchBatchSize.
	FetchBatchSize() int

	// SetWriteOpPoolSize sets the writeOperationPoolSize.
	SetWriteOpPoolSize(value int) Options

	// WriteOpPoolSize returns the writeOperationPoolSize.
	WriteOpPoolSize() int

	// SetWriteTaggedOpPoolSize sets the writeTaggedOperationPoolSize.
	SetWriteTaggedOpPoolSize(value int) Options

	// WriteTaggedOpPoolSize returns the writeTaggedOperationPoolSize.
	WriteTaggedOpPoolSize() int

	// SetFetchBatchOpPoolSize sets the fetchBatchOpPoolSize.
	SetFetchBatchOpPoolSize(value int) Options

	// FetchBatchOpPoolSize returns the fetchBatchOpPoolSize.
	FetchBatchOpPoolSize() int

	// SetCheckedBytesWrapperPoolSize sets the checkedBytesWrapperPoolSize.
	SetCheckedBytesWrapperPoolSize(value int) Options

	// CheckedBytesWrapperPoolSize returns the checkedBytesWrapperPoolSize.
	CheckedBytesWrapperPoolSize() int

	// SetHostQueueOpsFlushSize sets the hostQueueOpsFlushSize.
	SetHostQueueOpsFlushSize(value int) Options

	// HostQueueOpsFlushSize returns the hostQueueOpsFlushSize.
	HostQueueOpsFlushSize() int

	// SetHostQueueOpsFlushInterval sets the hostQueueOpsFlushInterval.
	SetHostQueueOpsFlushInterval(value time.Duration) Options

	// HostQueueOpsFlushInterval returns the hostQueueOpsFlushInterval.
	HostQueueOpsFlushInterval() time.Duration

	// SetContextPool sets the contextPool.
	SetContextPool(value context.Pool) Options

	// ContextPool returns the contextPool.
	ContextPool() context.Pool

	// SetIdentifierPool sets the identifier pool.
	SetIdentifierPool(value ident.Pool) Options

	// IdentifierPool returns the identifier pool.
	IdentifierPool() ident.Pool

	// HostQueueOpsArrayPoolSize sets the hostQueueOpsArrayPoolSize.
	SetHostQueueOpsArrayPoolSize(value int) Options

	// HostQueueOpsArrayPoolSize returns the hostQueueOpsArrayPoolSize.
	HostQueueOpsArrayPoolSize() int

	// SetHostQueueEmitsHealthStatus sets the hostQueueEmitHealthStatus.
	SetHostQueueEmitsHealthStatus(value bool) Options

	// HostQueueEmitsHealthStatus returns the hostQueueEmitHealthStatus.
	HostQueueEmitsHealthStatus() bool

	// SetSeriesIteratorPoolSize sets the seriesIteratorPoolSize.
	SetSeriesIteratorPoolSize(value int) Options

	// SeriesIteratorPoolSize returns the seriesIteratorPoolSize.
	SeriesIteratorPoolSize() int

	// SetSeriesIteratorArrayPoolBuckets sets the seriesIteratorArrayPoolBuckets.
	SetSeriesIteratorArrayPoolBuckets(value []pool.Bucket) Options

	// SeriesIteratorArrayPoolBuckets returns the seriesIteratorArrayPoolBuckets.
	SeriesIteratorArrayPoolBuckets() []pool.Bucket

	// SetReaderIteratorAllocate sets the readerIteratorAllocate.
	SetReaderIteratorAllocate(value encoding.ReaderIteratorAllocate) Options

	// ReaderIteratorAllocate returns the readerIteratorAllocate.
	ReaderIteratorAllocate() encoding.ReaderIteratorAllocate

	// SetSchemaRegistry sets the schema registry.
	SetSchemaRegistry(registry namespace.SchemaRegistry) AdminOptions

	// SchemaRegistry returns the schema registry.
	SchemaRegistry() namespace.SchemaRegistry

	// SetAsyncTopologyInitializers sets the AsyncTopologyInitializers
	SetAsyncTopologyInitializers(value []topology.Initializer) Options

	// AsyncTopologyInitializers returns the AsyncTopologyInitializers
	AsyncTopologyInitializers() []topology.Initializer

	// SetAsyncWriteWorkerPool sets the worker pool for async writes.
	SetAsyncWriteWorkerPool(value xsync.PooledWorkerPool) Options

	// AsyncWriteWorkerPool returns the worker pool for async writes.
	AsyncWriteWorkerPool() xsync.PooledWorkerPool

	// SetAsyncWriteMaxConcurrency sets the async writes maximum concurrency.
	SetAsyncWriteMaxConcurrency(value int) Options

	// AsyncWriteMaxConcurrency returns the async writes maximum concurrency.
	AsyncWriteMaxConcurrency() int

	// SetUseV2BatchAPIs sets whether the V2 batch APIs should be used.
	SetUseV2BatchAPIs(value bool) Options

	// UseV2BatchAPIs returns whether the V2 batch APIs should be used.
	UseV2BatchAPIs() bool

	// SetIterationOptions sets experimental iteration options.
	SetIterationOptions(index.IterationOptions) Options

	// IterationOptions returns experimental iteration options.
	IterationOptions() index.IterationOptions

	// SetWriteTimestampOffset sets the write timestamp offset.
	SetWriteTimestampOffset(value time.Duration) AdminOptions

	// WriteTimestampOffset returns the write timestamp offset.
	WriteTimestampOffset() time.Duration

	// SetNewConnectionFn sets a new connection generator function.
	SetNewConnectionFn(value NewConnectionFn) AdminOptions

	// NewConnectionFn returns the new connection generator function.
	NewConnectionFn() NewConnectionFn
}

// AdminOptions is a set of administration client options.
type AdminOptions interface {
	Options

	// SetOrigin sets the current host originating requests from.
	SetOrigin(value topology.Host) AdminOptions

	// Origin gets the current host originating requests from.
	Origin() topology.Host

	// SetBootstrapConsistencyLevel sets the bootstrap consistency level.
	SetBootstrapConsistencyLevel(value topology.ReadConsistencyLevel) AdminOptions

	// BootstrapConsistencyLevel returns the bootstrap consistency level.
	BootstrapConsistencyLevel() topology.ReadConsistencyLevel

	// SetFetchSeriesBlocksMaxBlockRetries sets the max retries for fetching series blocks.
	SetFetchSeriesBlocksMaxBlockRetries(value int) AdminOptions

	// FetchSeriesBlocksMaxBlockRetries gets the max retries for fetching series blocks.
	FetchSeriesBlocksMaxBlockRetries() int

	// SetFetchSeriesBlocksBatchSize sets the batch size for fetching series blocks in batch.
	SetFetchSeriesBlocksBatchSize(value int) AdminOptions

	// FetchSeriesBlocksBatchSize gets the batch size for fetching series blocks in batch.
	FetchSeriesBlocksBatchSize() int

	// SetFetchSeriesBlocksMetadataBatchTimeout sets the timeout for fetching series blocks metadata in batch.
	SetFetchSeriesBlocksMetadataBatchTimeout(value time.Duration) AdminOptions

	// FetchSeriesBlocksMetadataBatchTimeout gets the timeout for fetching series blocks metadata in batch.
	FetchSeriesBlocksMetadataBatchTimeout() time.Duration

	// SetFetchSeriesBlocksBatchTimeout sets the timeout for fetching series blocks in batch.
	SetFetchSeriesBlocksBatchTimeout(value time.Duration) AdminOptions

	// FetchSeriesBlocksBatchTimeout gets the timeout for fetching series blocks in batch.
	FetchSeriesBlocksBatchTimeout() time.Duration

	// SetFetchSeriesBlocksBatchConcurrency sets the concurrency for fetching series blocks in batch.
	SetFetchSeriesBlocksBatchConcurrency(value int) AdminOptions

	// FetchSeriesBlocksBatchConcurrency gets the concurrency for fetching series blocks in batch.
	FetchSeriesBlocksBatchConcurrency() int

	// SetStreamBlocksRetrier sets the retrier for streaming blocks.
	SetStreamBlocksRetrier(value xretry.Retrier) AdminOptions

	// StreamBlocksRetrier returns the retrier for streaming blocks.
	StreamBlocksRetrier() xretry.Retrier
}

// The rest of these types are internal types that mocks are generated for
// in file mode and hence need to stay in this file and refer to the other
// types such as AdminSession.  When mocks are generated in file mode the
// other types they reference need to be in the same file.

type clientSession interface {
	AdminSession

	// Open the client session.
	Open() error
}

type hostQueue interface {
	// Open the host queue.
	Open()

	// Len returns the length of the queue.
	Len() int

	// Enqueue an operation.
	Enqueue(op op) error

	// Host gets the host.
	Host() topology.Host

	// ConnectionCount gets the current open connection count.
	ConnectionCount() int

	// ConnectionPool gets the connection pool.
	ConnectionPool() connectionPool

	// BorrowConnection will borrow a connection and execute a user function.
	BorrowConnection(fn withConnectionFn) error

	// Close the host queue, will flush any operations still pending.
	Close()
}

type withConnectionFn func(c rpc.TChanNode)

type connectionPool interface {
	// Open starts the connection pool connecting and health checking.
	Open()

	// ConnectionCount gets the current open connection count.
	ConnectionCount() int

	// NextClient gets the next client for use by the connection pool.
	NextClient() (rpc.TChanNode, error)

	// Close the connection pool.
	Close()
}

type peerSource interface {
	// BorrowConnection will borrow a connection and execute a user function.
	BorrowConnection(hostID string, fn withConnectionFn) error
}

type peer interface {
	// Host gets the host.
	Host() topology.Host

	// BorrowConnection will borrow a connection and execute a user function.
	BorrowConnection(fn withConnectionFn) error
}

type status int

const (
	statusNotOpen status = iota
	statusOpen
	statusClosed
)

type healthStatus int

const (
	healthStatusCheckFailed healthStatus = iota
	healthStatusOK
)

type op interface {
	// Size returns the effective size of inner operations.
	Size() int

	// CompletionFn gets the completion function for the operation.
	CompletionFn() completionFn
}

type enqueueDelayedFn func(peersMetadata []receivedBlockMetadata)
type enqueueDelayedDoneFn func()

type enqueueChannel interface {
	enqueue(peersMetadata []receivedBlockMetadata) error
	enqueueDelayed(numToEnqueue int) (enqueueDelayedFn, enqueueDelayedDoneFn, error)
	// read is always safe to call since you can safely range
	// over a closed channel, and/or do a checked read in case
	// it is closed (unlike when publishing to a channel).
	read() <-chan []receivedBlockMetadata
	trackPending(amount int)
	trackProcessed(amount int)
	unprocessedLen() int
	closeOnAllProcessed()
}
