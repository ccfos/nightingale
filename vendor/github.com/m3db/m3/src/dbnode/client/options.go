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
	"errors"
	"io"
	"math"
	"runtime"
	"time"

	"github.com/m3db/m3/src/dbnode/clock"
	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/encoding/m3tsz"
	"github.com/m3db/m3/src/dbnode/encoding/proto"
	"github.com/m3db/m3/src/dbnode/environment"
	"github.com/m3db/m3/src/dbnode/generated/thrift/rpc"
	"github.com/m3db/m3/src/dbnode/namespace"
	nchannel "github.com/m3db/m3/src/dbnode/network/server/tchannelthrift/node/channel"
	m3dbruntime "github.com/m3db/m3/src/dbnode/runtime"
	"github.com/m3db/m3/src/dbnode/storage/index"
	"github.com/m3db/m3/src/dbnode/topology"
	xclose "github.com/m3db/m3/src/x/close"
	"github.com/m3db/m3/src/x/context"
	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/m3/src/x/instrument"
	"github.com/m3db/m3/src/x/pool"
	xretry "github.com/m3db/m3/src/x/retry"
	"github.com/m3db/m3/src/x/sampler"
	"github.com/m3db/m3/src/x/serialize"
	xsync "github.com/m3db/m3/src/x/sync"

	tchannel "github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
)

const (
	// DefaultWriteBatchSize is the default write and write tagged batch size.
	DefaultWriteBatchSize = 128

	// defaultWriteConsistencyLevel is the default write consistency level
	defaultWriteConsistencyLevel = m3dbruntime.DefaultWriteConsistencyLevel

	// defaultReadConsistencyLevel is the default read consistency level
	defaultReadConsistencyLevel = m3dbruntime.DefaultReadConsistencyLevel

	// defaultBootstrapConsistencyLevel is the default bootstrap consistency level
	defaultBootstrapConsistencyLevel = m3dbruntime.DefaultBootstrapConsistencyLevel

	// defaultMaxConnectionCount is the default max connection count
	defaultMaxConnectionCount = 32

	// defaultMinConnectionCount is the default min connection count
	defaultMinConnectionCount = 2

	// defaultHostConnectTimeout is the default host connection timeout
	defaultHostConnectTimeout = 5 * time.Second

	// defaultClusterConnectTimeout is the default cluster connect timeout
	defaultClusterConnectTimeout = 20 * time.Second

	// defaultClusterConnectConsistencyLevel is the default cluster connect consistency level
	defaultClusterConnectConsistencyLevel = topology.ConnectConsistencyLevelAny

	// defaultWriteRequestTimeout is the default write request timeout
	defaultWriteRequestTimeout = 10 * time.Second

	// defaultFetchRequestTimeout is the default fetch request timeout
	defaultFetchRequestTimeout = 15 * time.Second

	// defaultTruncateRequestTimeout is the default truncate request timeout
	defaultTruncateRequestTimeout = 60 * time.Second

	// defaultWriteShardsInitializing is the default write to shards intializing value
	defaultWriteShardsInitializing = true

	// defaultShardsLeavingCountTowardsConsistency is the default shards leaving count towards consistency
	defaultShardsLeavingCountTowardsConsistency = false

	// defaultIdentifierPoolSize is the default identifier pool size
	defaultIdentifierPoolSize = 8192

	// defaultWriteOpPoolSize is the default write op pool size
	defaultWriteOpPoolSize = 65536

	// defaultWriteTaggedOpPoolSize is the default write tagged op pool size
	defaultWriteTaggedOpPoolSize = 65536

	// defaultFetchBatchOpPoolSize is the default fetch op pool size
	defaultFetchBatchOpPoolSize = 1024

	// defaultFetchBatchSize is the default fetch batch size
	defaultFetchBatchSize = 128

	// defaultCheckedBytesWrapperPoolSize is the default checkedBytesWrapperPoolSize
	defaultCheckedBytesWrapperPoolSize = 65536

	// defaultHostQueueOpsFlushSize is the default host queue ops flush size
	defaultHostQueueOpsFlushSize = 128

	// defaultHostQueueOpsFlushInterval is the default host queue flush interval
	defaultHostQueueOpsFlushInterval = 5 * time.Millisecond

	// defaultHostQueueOpsArrayPoolSize is the default host queue ops array pool size
	defaultHostQueueOpsArrayPoolSize = 8

	// defaultHostQueueEmitsHealthStatus is false
	defaultHostQueueEmitsHealthStatus = false

	// defaultBackgroundConnectInterval is the default background connect interval
	defaultBackgroundConnectInterval = 4 * time.Second

	// defaultBackgroundConnectStutter is the default background connect stutter
	defaultBackgroundConnectStutter = 2 * time.Second

	// defaultBackgroundHealthCheckInterval is the default background health check interval
	defaultBackgroundHealthCheckInterval = 4 * time.Second

	// defaultBackgroundHealthCheckStutter is the default background health check stutter
	defaultBackgroundHealthCheckStutter = 2 * time.Second

	// defaultBackgroundHealthCheckFailLimit is the default background health failure
	// limit before connection is deemed unhealth
	defaultBackgroundHealthCheckFailLimit = 4

	// defaultBackgroundHealthCheckFailThrottleFactor is the default throttle factor to
	// apply when calculating how long to wait between a failed health check and a
	// retry attempt. It is applied by multiplying against the host connect
	// timeout to produce a throttle sleep value.
	defaultBackgroundHealthCheckFailThrottleFactor = 0.5

	// defaultSeriesIteratorPoolSize is the default size of the series iterator pools
	defaultSeriesIteratorPoolSize = 65536

	// defaultTagEncoderPoolSize is the default size of the tag encoder pool.
	defaultTagEncoderPoolSize = 4096

	// defaultTagDecoderPoolSize is the default size of the tag decoder pool.
	defaultTagDecoderPoolSize = 4096

	// defaultFetchSeriesBlocksMaxBlockRetries is the default max retries for fetch series blocks
	// from a single peer
	defaultFetchSeriesBlocksMaxBlockRetries = 2

	// defaultFetchSeriesBlocksBatchSize is the default fetch series blocks batch size
	defaultFetchSeriesBlocksBatchSize = 4096

	// defaultFetchSeriesBlocksMetadataBatchTimeout is the default series blocks metadata fetch timeout
	defaultFetchSeriesBlocksMetadataBatchTimeout = 60 * time.Second

	// defaultFetchSeriesBlocksMetadataBatchTimeout is the default series blocks contents fetch timeout
	defaultFetchSeriesBlocksBatchTimeout = 60 * time.Second

	// defaultAsyncWriteMaxConcurrency is the default maximum concurrency for async writes.
	defaultAsyncWriteMaxConcurrency = 4096

	// defaultUseV2BatchAPIs is the default setting for whether the v2 version of the batch APIs should
	// be used.
	defaultUseV2BatchAPIs = false
)

var (
	// defaultIdentifierPoolBytesPoolSizes is the default bytes pool sizes for the identifier pool
	defaultIdentifierPoolBytesPoolSizes = []pool.Bucket{
		{Capacity: 256, Count: defaultIdentifierPoolSize},
	}

	// defaultFetchSeriesBlocksBatchConcurrency is the default fetch series blocks in batch parallel concurrency limit
	defaultFetchSeriesBlocksBatchConcurrency = int(math.Max(1, float64(runtime.NumCPU())/2))

	// defaultSeriesIteratorArrayPoolBuckets is the default pool buckets for the series iterator array pool
	defaultSeriesIteratorArrayPoolBuckets = []pool.Bucket{}

	// defaulWriteRetrier is the default write retrier for write attempts
	defaultWriteRetrier = xretry.NewRetrier(
		xretry.NewOptions().
			SetInitialBackoff(500 * time.Millisecond).
			SetBackoffFactor(3).
			SetMaxRetries(2).
			SetJitter(true))

	// defaultFetchRetrier is the default fetch retrier for fetch attempts
	defaultFetchRetrier = xretry.NewRetrier(
		xretry.NewOptions().
			SetInitialBackoff(500 * time.Millisecond).
			SetBackoffFactor(2).
			SetMaxRetries(3).
			SetJitter(true))

	// defaultStreamBlocksRetrier is the default retrier for streaming blocks
	defaultStreamBlocksRetrier = xretry.NewRetrier(
		xretry.NewOptions().
			SetBackoffFactor(2).
			SetMaxRetries(3).
			SetInitialBackoff(2 * time.Second).
			SetJitter(true),
	)

	// defaultChannelOptions are default tchannel channel options.
	defaultChannelOptions = &tchannel.ChannelOptions{
		MaxIdleTime:       5 * time.Minute,
		IdleCheckInterval: 5 * time.Minute,
	}

	errNoTopologyInitializerSet    = errors.New("no topology initializer set")
	errNoReaderIteratorAllocateSet = errors.New("no reader iterator allocator set, encoding not set")
)

type options struct {
	runtimeOptsMgr                          m3dbruntime.OptionsManager
	clockOpts                               clock.Options
	instrumentOpts                          instrument.Options
	logErrorSampleRate                      sampler.Rate
	topologyInitializer                     topology.Initializer
	readConsistencyLevel                    topology.ReadConsistencyLevel
	writeConsistencyLevel                   topology.ConsistencyLevel
	bootstrapConsistencyLevel               topology.ReadConsistencyLevel
	channelOptions                          *tchannel.ChannelOptions
	maxConnectionCount                      int
	minConnectionCount                      int
	hostConnectTimeout                      time.Duration
	clusterConnectTimeout                   time.Duration
	clusterConnectConsistencyLevel          topology.ConnectConsistencyLevel
	writeRequestTimeout                     time.Duration
	fetchRequestTimeout                     time.Duration
	truncateRequestTimeout                  time.Duration
	backgroundConnectInterval               time.Duration
	backgroundConnectStutter                time.Duration
	backgroundHealthCheckInterval           time.Duration
	backgroundHealthCheckStutter            time.Duration
	backgroundHealthCheckFailLimit          int
	backgroundHealthCheckFailThrottleFactor float64
	tagEncoderOpts                          serialize.TagEncoderOptions
	tagEncoderPoolSize                      int
	tagDecoderOpts                          serialize.TagDecoderOptions
	tagDecoderPoolSize                      int
	writeRetrier                            xretry.Retrier
	fetchRetrier                            xretry.Retrier
	streamBlocksRetrier                     xretry.Retrier
	writeShardsInitializing                 bool
	shardsLeavingCountTowardsConsistency    bool
	newConnectionFn                         NewConnectionFn
	readerIteratorAllocate                  encoding.ReaderIteratorAllocate
	writeOperationPoolSize                  int
	writeTaggedOperationPoolSize            int
	fetchBatchOpPoolSize                    int
	writeBatchSize                          int
	fetchBatchSize                          int
	identifierPool                          ident.Pool
	hostQueueOpsFlushSize                   int
	hostQueueOpsFlushInterval               time.Duration
	hostQueueOpsArrayPoolSize               int
	hostQueueEmitsHealthStatus              bool
	seriesIteratorPoolSize                  int
	seriesIteratorArrayPoolBuckets          []pool.Bucket
	checkedBytesWrapperPoolSize             int
	contextPool                             context.Pool
	origin                                  topology.Host
	fetchSeriesBlocksMaxBlockRetries        int
	fetchSeriesBlocksBatchSize              int
	fetchSeriesBlocksMetadataBatchTimeout   time.Duration
	fetchSeriesBlocksBatchTimeout           time.Duration
	fetchSeriesBlocksBatchConcurrency       int
	schemaRegistry                          namespace.SchemaRegistry
	isProtoEnabled                          bool
	asyncTopologyInitializers               []topology.Initializer
	asyncWriteWorkerPool                    xsync.PooledWorkerPool
	asyncWriteMaxConcurrency                int
	useV2BatchAPIs                          bool
	iterationOptions                        index.IterationOptions
	writeTimestampOffset                    time.Duration
}

// NewOptions creates a new set of client options with defaults
func NewOptions() Options {
	return newOptions()
}

// NewAdminOptions creates a new set of administration client options with defaults
func NewAdminOptions() AdminOptions {
	return newOptions()
}

// NewOptionsForAsyncClusters returns a slice of Options, where each is the set of client
// for a given async client.
func NewOptionsForAsyncClusters(opts Options, topoInits []topology.Initializer, overrides []environment.ClientOverrides) []Options {
	result := make([]Options, 0, len(opts.AsyncTopologyInitializers()))
	for i, topoInit := range topoInits {
		options := opts.SetTopologyInitializer(topoInit)
		if overrides[i].HostQueueFlushInterval != nil {
			options = options.SetHostQueueOpsFlushInterval(*overrides[i].HostQueueFlushInterval)
		}
		if overrides[i].TargetHostQueueFlushSize != nil {
			options = options.SetHostQueueOpsFlushSize(*overrides[i].TargetHostQueueFlushSize)
		}
		result = append(result, options)
	}
	return result
}

func defaultNewConnectionFn(
	channelName string, address string, opts Options,
) (xclose.SimpleCloser, rpc.TChanNode, error) {
	channel, err := tchannel.NewChannel(channelName, opts.ChannelOptions())
	if err != nil {
		return nil, nil, err
	}
	endpoint := &thrift.ClientOptions{HostPort: address}
	thriftClient := thrift.NewClient(channel, nchannel.ChannelName, endpoint)
	client := rpc.NewTChanNodeClient(thriftClient)
	return channel, client, nil
}

func newOptions() *options {
	buckets := defaultIdentifierPoolBytesPoolSizes
	bytesPool := pool.NewCheckedBytesPool(buckets, nil,
		func(sizes []pool.Bucket) pool.BytesPool {
			return pool.NewBytesPool(sizes, nil)
		})
	bytesPool.Init()

	poolOpts := pool.NewObjectPoolOptions().
		SetSize(defaultIdentifierPoolSize)

	idPool := ident.NewPool(bytesPool, ident.PoolOptions{
		IDPoolOptions:           poolOpts,
		TagsPoolOptions:         poolOpts,
		TagsIteratorPoolOptions: poolOpts,
	})

	contextPool := context.NewPool(context.NewOptions().
		SetContextPoolOptions(poolOpts).
		SetFinalizerPoolOptions(poolOpts))

	opts := &options{
		clockOpts:                               clock.NewOptions(),
		instrumentOpts:                          instrument.NewOptions(),
		channelOptions:                          defaultChannelOptions,
		writeConsistencyLevel:                   defaultWriteConsistencyLevel,
		readConsistencyLevel:                    defaultReadConsistencyLevel,
		bootstrapConsistencyLevel:               defaultBootstrapConsistencyLevel,
		maxConnectionCount:                      defaultMaxConnectionCount,
		minConnectionCount:                      defaultMinConnectionCount,
		hostConnectTimeout:                      defaultHostConnectTimeout,
		clusterConnectTimeout:                   defaultClusterConnectTimeout,
		clusterConnectConsistencyLevel:          defaultClusterConnectConsistencyLevel,
		writeRequestTimeout:                     defaultWriteRequestTimeout,
		fetchRequestTimeout:                     defaultFetchRequestTimeout,
		truncateRequestTimeout:                  defaultTruncateRequestTimeout,
		backgroundConnectInterval:               defaultBackgroundConnectInterval,
		backgroundConnectStutter:                defaultBackgroundConnectStutter,
		backgroundHealthCheckInterval:           defaultBackgroundHealthCheckInterval,
		backgroundHealthCheckStutter:            defaultBackgroundHealthCheckStutter,
		backgroundHealthCheckFailLimit:          defaultBackgroundHealthCheckFailLimit,
		backgroundHealthCheckFailThrottleFactor: defaultBackgroundHealthCheckFailThrottleFactor,
		writeRetrier:                            defaultWriteRetrier,
		fetchRetrier:                            defaultFetchRetrier,
		writeShardsInitializing:                 defaultWriteShardsInitializing,
		shardsLeavingCountTowardsConsistency:    defaultShardsLeavingCountTowardsConsistency,
		tagEncoderPoolSize:                      defaultTagEncoderPoolSize,
		tagEncoderOpts:                          serialize.NewTagEncoderOptions(),
		tagDecoderPoolSize:                      defaultTagDecoderPoolSize,
		tagDecoderOpts:                          serialize.NewTagDecoderOptions(serialize.TagDecoderOptionsConfig{}),
		streamBlocksRetrier:                     defaultStreamBlocksRetrier,
		newConnectionFn:                         defaultNewConnectionFn,
		writeOperationPoolSize:                  defaultWriteOpPoolSize,
		writeTaggedOperationPoolSize:            defaultWriteTaggedOpPoolSize,
		fetchBatchOpPoolSize:                    defaultFetchBatchOpPoolSize,
		writeBatchSize:                          DefaultWriteBatchSize,
		fetchBatchSize:                          defaultFetchBatchSize,
		identifierPool:                          idPool,
		hostQueueOpsFlushSize:                   defaultHostQueueOpsFlushSize,
		hostQueueOpsFlushInterval:               defaultHostQueueOpsFlushInterval,
		hostQueueOpsArrayPoolSize:               defaultHostQueueOpsArrayPoolSize,
		hostQueueEmitsHealthStatus:              defaultHostQueueEmitsHealthStatus,
		seriesIteratorPoolSize:                  defaultSeriesIteratorPoolSize,
		seriesIteratorArrayPoolBuckets:          defaultSeriesIteratorArrayPoolBuckets,
		checkedBytesWrapperPoolSize:             defaultCheckedBytesWrapperPoolSize,
		contextPool:                             contextPool,
		fetchSeriesBlocksMaxBlockRetries:        defaultFetchSeriesBlocksMaxBlockRetries,
		fetchSeriesBlocksBatchSize:              defaultFetchSeriesBlocksBatchSize,
		fetchSeriesBlocksMetadataBatchTimeout:   defaultFetchSeriesBlocksMetadataBatchTimeout,
		fetchSeriesBlocksBatchTimeout:           defaultFetchSeriesBlocksBatchTimeout,
		fetchSeriesBlocksBatchConcurrency:       defaultFetchSeriesBlocksBatchConcurrency,
		schemaRegistry:                          namespace.NewSchemaRegistry(false, nil),
		asyncTopologyInitializers:               []topology.Initializer{},
		asyncWriteMaxConcurrency:                defaultAsyncWriteMaxConcurrency,
		useV2BatchAPIs:                          defaultUseV2BatchAPIs,
	}
	return opts.SetEncodingM3TSZ().(*options)
}

func validate(opts *options) error {
	if opts.topologyInitializer == nil {
		return errNoTopologyInitializerSet
	}
	if opts.readerIteratorAllocate == nil {
		return errNoReaderIteratorAllocateSet
	}
	if err := topology.ValidateConsistencyLevel(
		opts.writeConsistencyLevel,
	); err != nil {
		return err
	}
	if err := topology.ValidateReadConsistencyLevel(
		opts.readConsistencyLevel,
	); err != nil {
		return err
	}
	if err := topology.ValidateReadConsistencyLevel(
		opts.bootstrapConsistencyLevel,
	); err != nil {
		return err
	}
	if err := topology.ValidateConnectConsistencyLevel(
		opts.clusterConnectConsistencyLevel,
	); err != nil {
		return err
	}
	return opts.logErrorSampleRate.Validate()
}

func (o *options) Validate() error {
	return validate(o)
}

func (o *options) SetEncodingM3TSZ() Options {
	opts := *o
	opts.readerIteratorAllocate = func(r io.Reader, _ namespace.SchemaDescr) encoding.ReaderIterator {
		return m3tsz.NewReaderIterator(r, m3tsz.DefaultIntOptimizationEnabled, encoding.NewOptions())
	}
	opts.isProtoEnabled = false
	return &opts
}

func (o *options) SetEncodingProto(encodingOpts encoding.Options) Options {
	opts := *o
	opts.readerIteratorAllocate = func(r io.Reader, descr namespace.SchemaDescr) encoding.ReaderIterator {
		return proto.NewIterator(r, descr, encodingOpts)
	}
	opts.isProtoEnabled = true
	return &opts
}

func (o *options) IsSetEncodingProto() bool {
	return o.isProtoEnabled
}

func (o *options) SetRuntimeOptionsManager(value m3dbruntime.OptionsManager) Options {
	opts := *o
	opts.runtimeOptsMgr = value
	return &opts
}

func (o *options) RuntimeOptionsManager() m3dbruntime.OptionsManager {
	return o.runtimeOptsMgr
}

func (o *options) SetClockOptions(value clock.Options) Options {
	opts := *o
	opts.clockOpts = value
	return &opts
}

func (o *options) ClockOptions() clock.Options {
	return o.clockOpts
}

func (o *options) SetInstrumentOptions(value instrument.Options) Options {
	opts := *o
	opts.instrumentOpts = value
	return &opts
}

func (o *options) InstrumentOptions() instrument.Options {
	return o.instrumentOpts
}

func (o *options) SetLogErrorSampleRate(value sampler.Rate) Options {
	opts := *o
	opts.logErrorSampleRate = value
	return &opts
}

func (o *options) LogErrorSampleRate() sampler.Rate {
	return o.logErrorSampleRate
}

func (o *options) SetTopologyInitializer(value topology.Initializer) Options {
	opts := *o
	opts.topologyInitializer = value
	return &opts
}

func (o *options) TopologyInitializer() topology.Initializer {
	return o.topologyInitializer
}

func (o *options) SetReadConsistencyLevel(value topology.ReadConsistencyLevel) Options {
	opts := *o
	opts.readConsistencyLevel = value
	return &opts
}

func (o *options) ReadConsistencyLevel() topology.ReadConsistencyLevel {
	return o.readConsistencyLevel
}

func (o *options) SetWriteConsistencyLevel(value topology.ConsistencyLevel) Options {
	opts := *o
	opts.writeConsistencyLevel = value
	return &opts
}

func (o *options) WriteConsistencyLevel() topology.ConsistencyLevel {
	return o.writeConsistencyLevel
}

func (o *options) SetBootstrapConsistencyLevel(value topology.ReadConsistencyLevel) AdminOptions {
	opts := *o
	opts.bootstrapConsistencyLevel = value
	return &opts
}

func (o *options) BootstrapConsistencyLevel() topology.ReadConsistencyLevel {
	return o.bootstrapConsistencyLevel
}

func (o *options) SetChannelOptions(value *tchannel.ChannelOptions) Options {
	opts := *o
	opts.channelOptions = value
	return &opts
}

func (o *options) ChannelOptions() *tchannel.ChannelOptions {
	return o.channelOptions
}

func (o *options) SetMaxConnectionCount(value int) Options {
	opts := *o
	opts.maxConnectionCount = value
	return &opts
}

func (o *options) MaxConnectionCount() int {
	return o.maxConnectionCount
}

func (o *options) SetMinConnectionCount(value int) Options {
	opts := *o
	opts.minConnectionCount = value
	return &opts
}

func (o *options) MinConnectionCount() int {
	return o.minConnectionCount
}

func (o *options) SetHostConnectTimeout(value time.Duration) Options {
	opts := *o
	opts.hostConnectTimeout = value
	return &opts
}

func (o *options) HostConnectTimeout() time.Duration {
	return o.hostConnectTimeout
}

func (o *options) SetClusterConnectTimeout(value time.Duration) Options {
	opts := *o
	opts.clusterConnectTimeout = value
	return &opts
}

func (o *options) ClusterConnectTimeout() time.Duration {
	return o.clusterConnectTimeout
}

func (o *options) SetClusterConnectConsistencyLevel(value topology.ConnectConsistencyLevel) Options {
	opts := *o
	opts.clusterConnectConsistencyLevel = value
	return &opts
}

func (o *options) ClusterConnectConsistencyLevel() topology.ConnectConsistencyLevel {
	return o.clusterConnectConsistencyLevel
}

func (o *options) SetWriteRequestTimeout(value time.Duration) Options {
	opts := *o
	opts.writeRequestTimeout = value
	return &opts
}

func (o *options) WriteRequestTimeout() time.Duration {
	return o.writeRequestTimeout
}

func (o *options) SetFetchRequestTimeout(value time.Duration) Options {
	opts := *o
	opts.fetchRequestTimeout = value
	return &opts
}

func (o *options) FetchRequestTimeout() time.Duration {
	return o.fetchRequestTimeout
}

func (o *options) SetTruncateRequestTimeout(value time.Duration) Options {
	opts := *o
	opts.truncateRequestTimeout = value
	return &opts
}

func (o *options) TruncateRequestTimeout() time.Duration {
	return o.truncateRequestTimeout
}

func (o *options) SetBackgroundConnectInterval(value time.Duration) Options {
	opts := *o
	opts.backgroundConnectInterval = value
	return &opts
}

func (o *options) BackgroundConnectInterval() time.Duration {
	return o.writeRequestTimeout
}

func (o *options) SetBackgroundConnectStutter(value time.Duration) Options {
	opts := *o
	opts.backgroundConnectStutter = value
	return &opts
}

func (o *options) BackgroundConnectStutter() time.Duration {
	return o.backgroundConnectStutter
}

func (o *options) SetBackgroundHealthCheckInterval(value time.Duration) Options {
	opts := *o
	opts.backgroundHealthCheckInterval = value
	return &opts
}

func (o *options) BackgroundHealthCheckInterval() time.Duration {
	return o.backgroundHealthCheckInterval
}

func (o *options) SetBackgroundHealthCheckStutter(value time.Duration) Options {
	opts := *o
	opts.backgroundHealthCheckStutter = value
	return &opts
}

func (o *options) BackgroundHealthCheckStutter() time.Duration {
	return o.backgroundHealthCheckStutter
}

func (o *options) SetBackgroundHealthCheckFailLimit(value int) Options {
	opts := *o
	opts.backgroundHealthCheckFailLimit = value
	return &opts
}

func (o *options) BackgroundHealthCheckFailLimit() int {
	return o.backgroundHealthCheckFailLimit
}

func (o *options) SetBackgroundHealthCheckFailThrottleFactor(value float64) Options {
	opts := *o
	opts.backgroundHealthCheckFailThrottleFactor = value
	return &opts
}

func (o *options) BackgroundHealthCheckFailThrottleFactor() float64 {
	return o.backgroundHealthCheckFailThrottleFactor
}

func (o *options) SetWriteRetrier(value xretry.Retrier) Options {
	opts := *o
	opts.writeRetrier = value
	return &opts
}

func (o *options) WriteRetrier() xretry.Retrier {
	return o.writeRetrier
}

func (o *options) SetFetchRetrier(value xretry.Retrier) Options {
	opts := *o
	opts.fetchRetrier = value
	return &opts
}

func (o *options) FetchRetrier() xretry.Retrier {
	return o.fetchRetrier
}

func (o *options) SetWriteShardsInitializing(value bool) Options {
	opts := *o
	opts.writeShardsInitializing = value
	return &opts
}

func (o *options) WriteShardsInitializing() bool {
	return o.writeShardsInitializing
}

func (o *options) SetShardsLeavingCountTowardsConsistency(value bool) Options {
	opts := *o
	opts.shardsLeavingCountTowardsConsistency = value
	return &opts
}

func (o *options) ShardsLeavingCountTowardsConsistency() bool {
	return o.shardsLeavingCountTowardsConsistency
}

func (o *options) SetTagEncoderOptions(value serialize.TagEncoderOptions) Options {
	opts := *o
	opts.tagEncoderOpts = value
	return &opts
}

func (o *options) TagEncoderOptions() serialize.TagEncoderOptions {
	return o.tagEncoderOpts
}

func (o *options) SetTagEncoderPoolSize(value int) Options {
	opts := *o
	opts.tagEncoderPoolSize = value
	return &opts
}

func (o *options) TagEncoderPoolSize() int {
	return o.tagEncoderPoolSize
}

func (o *options) SetTagDecoderOptions(value serialize.TagDecoderOptions) Options {
	opts := *o
	opts.tagDecoderOpts = value
	return &opts
}

func (o *options) TagDecoderOptions() serialize.TagDecoderOptions {
	return o.tagDecoderOpts
}

func (o *options) SetTagDecoderPoolSize(value int) Options {
	opts := *o
	opts.tagDecoderPoolSize = value
	return &opts
}

func (o *options) TagDecoderPoolSize() int {
	return o.tagDecoderPoolSize
}

func (o *options) SetStreamBlocksRetrier(value xretry.Retrier) AdminOptions {
	opts := *o
	opts.streamBlocksRetrier = value
	return &opts
}

func (o *options) StreamBlocksRetrier() xretry.Retrier {
	return o.streamBlocksRetrier
}

func (o *options) SetNewConnectionFn(value NewConnectionFn) AdminOptions {
	opts := *o
	opts.newConnectionFn = value
	return &opts
}

func (o *options) NewConnectionFn() NewConnectionFn {
	return o.newConnectionFn
}

func (o *options) SetWriteOpPoolSize(value int) Options {
	opts := *o
	opts.writeOperationPoolSize = value
	return &opts
}

func (o *options) WriteOpPoolSize() int {
	return o.writeOperationPoolSize
}

func (o *options) SetWriteTaggedOpPoolSize(value int) Options {
	opts := *o
	opts.writeTaggedOperationPoolSize = value
	return &opts
}

func (o *options) WriteTaggedOpPoolSize() int {
	return o.writeTaggedOperationPoolSize
}

func (o *options) SetFetchBatchOpPoolSize(value int) Options {
	opts := *o
	opts.fetchBatchOpPoolSize = value
	return &opts
}

func (o *options) FetchBatchOpPoolSize() int {
	return o.fetchBatchOpPoolSize
}

func (o *options) SetContextPool(value context.Pool) Options {
	opts := *o
	opts.contextPool = value
	return &opts
}

func (o *options) ContextPool() context.Pool {
	return o.contextPool
}

func (o *options) SetWriteBatchSize(value int) Options {
	opts := *o
	opts.writeBatchSize = value
	return &opts
}

func (o *options) WriteBatchSize() int {
	return o.writeBatchSize
}

func (o *options) SetFetchBatchSize(value int) Options {
	opts := *o
	opts.fetchBatchSize = value
	return &opts
}

func (o *options) FetchBatchSize() int {
	return o.fetchBatchSize
}

func (o *options) SetIdentifierPool(value ident.Pool) Options {
	opts := *o
	opts.identifierPool = value
	return &opts
}

func (o *options) IdentifierPool() ident.Pool {
	return o.identifierPool
}

func (o *options) SetCheckedBytesWrapperPoolSize(value int) Options {
	opts := *o
	opts.checkedBytesWrapperPoolSize = value
	return &opts
}

func (o *options) CheckedBytesWrapperPoolSize() int {
	return o.checkedBytesWrapperPoolSize
}

func (o *options) SetHostQueueOpsFlushSize(value int) Options {
	opts := *o
	opts.hostQueueOpsFlushSize = value
	return &opts
}

func (o *options) HostQueueOpsFlushSize() int {
	return o.hostQueueOpsFlushSize
}

func (o *options) SetHostQueueOpsFlushInterval(value time.Duration) Options {
	opts := *o
	opts.hostQueueOpsFlushInterval = value
	return &opts
}

func (o *options) HostQueueOpsFlushInterval() time.Duration {
	return o.hostQueueOpsFlushInterval
}

func (o *options) SetHostQueueOpsArrayPoolSize(value int) Options {
	opts := *o
	opts.hostQueueOpsArrayPoolSize = value
	return &opts
}

func (o *options) HostQueueOpsArrayPoolSize() int {
	return o.hostQueueOpsArrayPoolSize
}

func (o *options) SetHostQueueEmitsHealthStatus(value bool) Options {
	opts := *o
	opts.hostQueueEmitsHealthStatus = value
	return &opts
}

func (o *options) HostQueueEmitsHealthStatus() bool {
	return o.hostQueueEmitsHealthStatus
}

func (o *options) SetSeriesIteratorPoolSize(value int) Options {
	opts := *o
	opts.seriesIteratorPoolSize = value
	return &opts
}

func (o *options) SeriesIteratorPoolSize() int {
	return o.seriesIteratorPoolSize
}

func (o *options) SetSeriesIteratorArrayPoolBuckets(value []pool.Bucket) Options {
	opts := *o
	opts.seriesIteratorArrayPoolBuckets = value
	return &opts
}

func (o *options) SeriesIteratorArrayPoolBuckets() []pool.Bucket {
	return o.seriesIteratorArrayPoolBuckets
}

func (o *options) SetReaderIteratorAllocate(value encoding.ReaderIteratorAllocate) Options {
	opts := *o
	opts.readerIteratorAllocate = value
	return &opts
}

func (o *options) ReaderIteratorAllocate() encoding.ReaderIteratorAllocate {
	return o.readerIteratorAllocate
}

func (o *options) SetSchemaRegistry(registry namespace.SchemaRegistry) AdminOptions {
	opts := *o
	opts.schemaRegistry = registry
	return &opts
}

func (o *options) SchemaRegistry() namespace.SchemaRegistry {
	return o.schemaRegistry
}

func (o *options) SetOrigin(value topology.Host) AdminOptions {
	opts := *o
	opts.origin = value
	return &opts
}

func (o *options) Origin() topology.Host {
	return o.origin
}

func (o *options) SetFetchSeriesBlocksMaxBlockRetries(value int) AdminOptions {
	opts := *o
	opts.fetchSeriesBlocksMaxBlockRetries = value
	return &opts
}

func (o *options) FetchSeriesBlocksMaxBlockRetries() int {
	return o.fetchSeriesBlocksMaxBlockRetries
}

func (o *options) SetFetchSeriesBlocksBatchSize(value int) AdminOptions {
	opts := *o
	opts.fetchSeriesBlocksBatchSize = value
	return &opts
}

func (o *options) FetchSeriesBlocksBatchSize() int {
	return o.fetchSeriesBlocksBatchSize
}

func (o *options) SetFetchSeriesBlocksMetadataBatchTimeout(value time.Duration) AdminOptions {
	opts := *o
	opts.fetchSeriesBlocksMetadataBatchTimeout = value
	return &opts
}

func (o *options) FetchSeriesBlocksMetadataBatchTimeout() time.Duration {
	return o.fetchSeriesBlocksMetadataBatchTimeout
}

func (o *options) SetFetchSeriesBlocksBatchTimeout(value time.Duration) AdminOptions {
	opts := *o
	opts.fetchSeriesBlocksBatchTimeout = value
	return &opts
}

func (o *options) FetchSeriesBlocksBatchTimeout() time.Duration {
	return o.fetchSeriesBlocksBatchTimeout
}

func (o *options) SetFetchSeriesBlocksBatchConcurrency(value int) AdminOptions {
	opts := *o
	opts.fetchSeriesBlocksBatchConcurrency = value
	return &opts
}

func (o *options) FetchSeriesBlocksBatchConcurrency() int {
	return o.fetchSeriesBlocksBatchConcurrency
}

func (o *options) SetAsyncTopologyInitializers(value []topology.Initializer) Options {
	opts := *o
	opts.asyncTopologyInitializers = value
	return &opts
}

func (o *options) AsyncTopologyInitializers() []topology.Initializer {
	return o.asyncTopologyInitializers
}

func (o *options) SetAsyncWriteWorkerPool(value xsync.PooledWorkerPool) Options {
	opts := *o
	opts.asyncWriteWorkerPool = value
	return &opts
}

func (o *options) AsyncWriteWorkerPool() xsync.PooledWorkerPool {
	return o.asyncWriteWorkerPool
}

func (o *options) SetAsyncWriteMaxConcurrency(value int) Options {
	opts := *o
	opts.asyncWriteMaxConcurrency = value
	return &opts
}

func (o *options) AsyncWriteMaxConcurrency() int {
	return o.asyncWriteMaxConcurrency
}

func (o *options) SetUseV2BatchAPIs(value bool) Options {
	opts := *o
	opts.useV2BatchAPIs = value
	return &opts
}

func (o *options) UseV2BatchAPIs() bool {
	return o.useV2BatchAPIs
}

func (o *options) SetIterationOptions(value index.IterationOptions) Options {
	opts := *o
	opts.iterationOptions = value
	return &opts
}

func (o *options) IterationOptions() index.IterationOptions {
	return o.iterationOptions
}

func (o *options) SetWriteTimestampOffset(value time.Duration) AdminOptions {
	opts := *o
	opts.writeTimestampOffset = value
	return &opts
}

func (o *options) WriteTimestampOffset() time.Duration {
	return o.writeTimestampOffset
}
