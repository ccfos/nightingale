// Copyright (c) 2017 Uber Technologies, Inc.
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
	"fmt"
	"io"
	"time"

	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/encoding/m3tsz"
	"github.com/m3db/m3/src/dbnode/environment"
	"github.com/m3db/m3/src/dbnode/namespace"
	"github.com/m3db/m3/src/dbnode/topology"
	xerrors "github.com/m3db/m3/src/x/errors"
	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/m3/src/x/instrument"
	"github.com/m3db/m3/src/x/retry"
	"github.com/m3db/m3/src/x/sampler"
	xsync "github.com/m3db/m3/src/x/sync"
)

const (
	asyncWriteWorkerPoolDefaultSize = 128
)

var (
	errConfigurationMustSupplyConfig = errors.New(
		"must supply config when no topology initializer parameter supplied")
)

// Configuration is a configuration that can be used to construct a client.
type Configuration struct {
	// The environment (static or dynamic) configuration.
	EnvironmentConfig *environment.Configuration `yaml:"config"`

	// WriteConsistencyLevel specifies the write consistency level.
	WriteConsistencyLevel *topology.ConsistencyLevel `yaml:"writeConsistencyLevel"`

	// ReadConsistencyLevel specifies the read consistency level.
	ReadConsistencyLevel *topology.ReadConsistencyLevel `yaml:"readConsistencyLevel"`

	// ConnectConsistencyLevel specifies the cluster connect consistency level.
	ConnectConsistencyLevel *topology.ConnectConsistencyLevel `yaml:"connectConsistencyLevel"`

	// WriteTimeout is the write request timeout.
	WriteTimeout *time.Duration `yaml:"writeTimeout"`

	// FetchTimeout is the fetch request timeout.
	FetchTimeout *time.Duration `yaml:"fetchTimeout"`

	// ConnectTimeout is the cluster connect timeout.
	ConnectTimeout *time.Duration `yaml:"connectTimeout"`

	// WriteRetry is the write retry config.
	WriteRetry *retry.Configuration `yaml:"writeRetry"`

	// FetchRetry is the fetch retry config.
	FetchRetry *retry.Configuration `yaml:"fetchRetry"`

	// LogErrorSampleRate is the log error sample rate.
	LogErrorSampleRate sampler.Rate `yaml:"logErrorSampleRate"`

	// BackgroundHealthCheckFailLimit is the amount of times a background check
	// must fail before a connection is taken out of consideration.
	BackgroundHealthCheckFailLimit *int `yaml:"backgroundHealthCheckFailLimit"`

	// BackgroundHealthCheckFailThrottleFactor is the factor of the host connect
	// time to use when sleeping between a failed health check and the next check.
	BackgroundHealthCheckFailThrottleFactor *float64 `yaml:"backgroundHealthCheckFailThrottleFactor"`

	// HashingConfiguration is the configuration for hashing of IDs to shards.
	HashingConfiguration *HashingConfiguration `yaml:"hashing"`

	// Proto contains the configuration specific to running in the ProtoDataMode.
	Proto *ProtoConfiguration `yaml:"proto"`

	// AsyncWriteWorkerPoolSize is the worker pool size for async write requests.
	AsyncWriteWorkerPoolSize *int `yaml:"asyncWriteWorkerPoolSize"`

	// AsyncWriteMaxConcurrency is the maximum concurrency for async write requests.
	AsyncWriteMaxConcurrency *int `yaml:"asyncWriteMaxConcurrency"`

	// UseV2BatchAPIs determines whether the V2 batch APIs are used. Note that the M3DB nodes must
	// have support for the V2 APIs in order for this feature to be used.
	UseV2BatchAPIs *bool `yaml:"useV2BatchAPIs"`

	// WriteTimestampOffset offsets all writes by specified duration into the past.
	WriteTimestampOffset *time.Duration `yaml:"writeTimestampOffset"`

	// FetchSeriesBlocksBatchConcurrency sets the number of batches of blocks to retrieve
	// in parallel from a remote peer. Defaults to NumCPU / 2.
	FetchSeriesBlocksBatchConcurrency *int `yaml:"fetchSeriesBlocksBatchConcurrency"`

	// FetchSeriesBlocksBatchSize sets the number of blocks to retrieve in a single batch
	// from the remote peer. Defaults to 4096.
	FetchSeriesBlocksBatchSize *int `yaml:"fetchSeriesBlocksBatchSize"`

	// WriteShardsInitializing sets whether or not writes to leaving shards
	// count towards consistency, by default they do not.
	WriteShardsInitializing *bool `yaml:"writeShardsInitializing"`

	// ShardsLeavingCountTowardsConsistency sets whether or not writes to leaving shards
	// count towards consistency, by default they do not.
	ShardsLeavingCountTowardsConsistency *bool `yaml:"shardsLeavingCountTowardsConsistency"`
}

// ProtoConfiguration is the configuration for running with ProtoDataMode enabled.
type ProtoConfiguration struct {
	// Enabled specifies whether proto is enabled.
	Enabled bool `yaml:"enabled"`
	// load user schema from client configuration into schema registry
	// at startup/initialization time.
	SchemaRegistry map[string]NamespaceProtoSchema `yaml:"schema_registry"`
}

// NamespaceProtoSchema is the protobuf schema for a namespace.
type NamespaceProtoSchema struct {
	MessageName    string `yaml:"messageName"`
	SchemaDeployID string `yaml:"schemaDeployID"`
	SchemaFilePath string `yaml:"schemaFilePath"`
}

// Validate validates the NamespaceProtoSchema.
func (c NamespaceProtoSchema) Validate() error {
	if c.SchemaFilePath == "" {
		return errors.New("schemaFilePath is required for Proto data mode")
	}

	if c.MessageName == "" {
		return errors.New("messageName is required for Proto data mode")
	}

	return nil
}

// Validate validates the ProtoConfiguration.
func (c *ProtoConfiguration) Validate() error {
	if c == nil || !c.Enabled {
		return nil
	}

	for _, schema := range c.SchemaRegistry {
		if err := schema.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Validate validates the configuration.
func (c *Configuration) Validate() error {
	if c.WriteTimeout != nil && *c.WriteTimeout < 0 {
		return fmt.Errorf("m3db client writeTimeout was: %d but must be >= 0", *c.WriteTimeout)
	}

	if c.FetchTimeout != nil && *c.FetchTimeout < 0 {
		return fmt.Errorf("m3db client fetchTimeout was: %d but must be >= 0", *c.FetchTimeout)
	}

	if c.ConnectTimeout != nil && *c.ConnectTimeout < 0 {
		return fmt.Errorf("m3db client connectTimeout was: %d but must be >= 0", *c.ConnectTimeout)
	}

	if err := c.LogErrorSampleRate.Validate(); err != nil {
		return fmt.Errorf("m3db client error validating log error sample rate: %v", err)
	}

	if c.BackgroundHealthCheckFailLimit != nil &&
		(*c.BackgroundHealthCheckFailLimit < 0 || *c.BackgroundHealthCheckFailLimit > 10) {
		return fmt.Errorf(
			"m3db client backgroundHealthCheckFailLimit was: %d but must be >= 0 and <=10",
			*c.BackgroundHealthCheckFailLimit)
	}

	if c.BackgroundHealthCheckFailThrottleFactor != nil &&
		(*c.BackgroundHealthCheckFailThrottleFactor < 0 || *c.BackgroundHealthCheckFailThrottleFactor > 10) {
		return fmt.Errorf(
			"m3db client backgroundHealthCheckFailThrottleFactor was: %f but must be >= 0 and <=10",
			*c.BackgroundHealthCheckFailThrottleFactor)
	}

	if c.AsyncWriteWorkerPoolSize != nil && *c.AsyncWriteWorkerPoolSize <= 0 {
		return fmt.Errorf("m3db client async write worker pool size was: %d but must be >0",
			*c.AsyncWriteWorkerPoolSize)
	}

	if c.AsyncWriteMaxConcurrency != nil && *c.AsyncWriteMaxConcurrency <= 0 {
		return fmt.Errorf("m3db client async write max concurrency was: %d but must be >0",
			*c.AsyncWriteMaxConcurrency)
	}

	if err := c.Proto.Validate(); err != nil {
		return fmt.Errorf("error validating M3DB client proto configuration: %v", err)
	}

	return nil
}

// HashingConfiguration is the configuration for hashing
type HashingConfiguration struct {
	// Murmur32 seed value
	Seed uint32 `yaml:"seed"`
}

// ConfigurationParameters are optional parameters that can be specified
// when creating a client from configuration, this is specified using
// a struct so that adding fields do not cause breaking changes to callers.
type ConfigurationParameters struct {
	// InstrumentOptions is a required argument when
	// constructing a client from configuration.
	InstrumentOptions instrument.Options

	// TopologyInitializer is an optional argument when
	// constructing a client from configuration.
	TopologyInitializer topology.Initializer

	// EncodingOptions is an optional argument when
	// constructing a client from configuration.
	EncodingOptions encoding.Options
}

// CustomOption is a programatic method for setting a client
// option after all the options have been set by configuration.
type CustomOption func(v Options) Options

// CustomAdminOption is a programatic method for setting a client
// admin option after all the options have been set by configuration.
type CustomAdminOption func(v AdminOptions) AdminOptions

// NewClient creates a new M3DB client using
// specified params and custom options.
func (c Configuration) NewClient(
	params ConfigurationParameters,
	custom ...CustomOption,
) (Client, error) {
	customAdmin := make([]CustomAdminOption, 0, len(custom))
	for _, opt := range custom {
		customAdmin = append(customAdmin, func(v AdminOptions) AdminOptions {
			return opt(Options(v)).(AdminOptions)
		})
	}

	v, err := c.NewAdminClient(params, customAdmin...)
	if err != nil {
		return nil, err
	}

	return v, err
}

// NewAdminClient creates a new M3DB admin client using
// specified params and custom options.
func (c Configuration) NewAdminClient(
	params ConfigurationParameters,
	custom ...CustomAdminOption,
) (AdminClient, error) {
	err := c.Validate()
	if err != nil {
		return nil, err
	}

	iopts := params.InstrumentOptions
	if iopts == nil {
		iopts = instrument.NewOptions()
	}
	writeRequestScope := iopts.MetricsScope().SubScope("write-req")
	fetchRequestScope := iopts.MetricsScope().SubScope("fetch-req")

	cfgParams := environment.ConfigurationParameters{
		InstrumentOpts: iopts,
	}
	if c.HashingConfiguration != nil {
		cfgParams.HashingSeed = c.HashingConfiguration.Seed
	}

	var (
		syncTopoInit         = params.TopologyInitializer
		syncClientOverrides  environment.ClientOverrides
		asyncTopoInits       = []topology.Initializer{}
		asyncClientOverrides = []environment.ClientOverrides{}
	)

	var buildAsyncPool bool
	if syncTopoInit == nil {
		envCfgs, err := c.EnvironmentConfig.Configure(cfgParams)
		if err != nil {
			err = fmt.Errorf("unable to create topology initializer, err: %v", err)
			return nil, err
		}

		for _, envCfg := range envCfgs {
			if envCfg.Async {
				asyncTopoInits = append(asyncTopoInits, envCfg.TopologyInitializer)
				asyncClientOverrides = append(asyncClientOverrides, envCfg.ClientOverrides)
				buildAsyncPool = true
			} else {
				syncTopoInit = envCfg.TopologyInitializer
				syncClientOverrides = envCfg.ClientOverrides
			}
		}
	}

	v := NewAdminOptions().
		SetTopologyInitializer(syncTopoInit).
		SetAsyncTopologyInitializers(asyncTopoInits).
		SetInstrumentOptions(iopts).
		SetLogErrorSampleRate(c.LogErrorSampleRate)

	if c.UseV2BatchAPIs != nil {
		v = v.SetUseV2BatchAPIs(*c.UseV2BatchAPIs)
	}

	if buildAsyncPool {
		var size int
		if c.AsyncWriteWorkerPoolSize == nil {
			size = asyncWriteWorkerPoolDefaultSize
		} else {
			size = *c.AsyncWriteWorkerPoolSize
		}

		workerPoolInstrumentOpts := iopts.SetMetricsScope(writeRequestScope.SubScope("workerpool"))
		workerPoolOpts := xsync.NewPooledWorkerPoolOptions().
			SetGrowOnDemand(true).
			SetInstrumentOptions(workerPoolInstrumentOpts)
		workerPool, err := xsync.NewPooledWorkerPool(size, workerPoolOpts)
		if err != nil {
			return nil, fmt.Errorf("unable to create async worker pool: %v", err)
		}
		workerPool.Init()
		v = v.SetAsyncWriteWorkerPool(workerPool)
	}

	if c.AsyncWriteMaxConcurrency != nil {
		v = v.SetAsyncWriteMaxConcurrency(*c.AsyncWriteMaxConcurrency)
	}

	if c.WriteConsistencyLevel != nil {
		v = v.SetWriteConsistencyLevel(*c.WriteConsistencyLevel)
	}
	if c.ReadConsistencyLevel != nil {
		v = v.SetReadConsistencyLevel(*c.ReadConsistencyLevel)
	}
	if c.ConnectConsistencyLevel != nil {
		v.SetClusterConnectConsistencyLevel(*c.ConnectConsistencyLevel)
	}
	if c.BackgroundHealthCheckFailLimit != nil {
		v = v.SetBackgroundHealthCheckFailLimit(*c.BackgroundHealthCheckFailLimit)
	}
	if c.BackgroundHealthCheckFailThrottleFactor != nil {
		v = v.SetBackgroundHealthCheckFailThrottleFactor(*c.BackgroundHealthCheckFailThrottleFactor)
	}
	if c.WriteTimeout != nil {
		v = v.SetWriteRequestTimeout(*c.WriteTimeout)
	}
	if c.FetchTimeout != nil {
		v = v.SetFetchRequestTimeout(*c.FetchTimeout)
	}
	if c.ConnectTimeout != nil {
		v = v.SetClusterConnectTimeout(*c.ConnectTimeout)
	}
	if c.WriteRetry != nil {
		v = v.SetWriteRetrier(c.WriteRetry.NewRetrier(writeRequestScope))
	} else {
		// Have not set write retry explicitly, but would like metrics
		// emitted for the write retrier with the scope for write requests.
		retrierOpts := v.WriteRetrier().Options().
			SetMetricsScope(writeRequestScope)
		v = v.SetWriteRetrier(retry.NewRetrier(retrierOpts))
	}
	if c.FetchRetry != nil {
		v = v.SetFetchRetrier(c.FetchRetry.NewRetrier(fetchRequestScope))
	} else {
		// Have not set fetch retry explicitly, but would like metrics
		// emitted for the fetch retrier with the scope for fetch requests.
		retrierOpts := v.FetchRetrier().Options().
			SetMetricsScope(fetchRequestScope)
		v = v.SetFetchRetrier(retry.NewRetrier(retrierOpts))
	}
	if syncClientOverrides.TargetHostQueueFlushSize != nil {
		v = v.SetHostQueueOpsFlushSize(*syncClientOverrides.TargetHostQueueFlushSize)
	}
	if syncClientOverrides.HostQueueFlushInterval != nil {
		v = v.SetHostQueueOpsFlushInterval(*syncClientOverrides.HostQueueFlushInterval)
	}

	encodingOpts := params.EncodingOptions
	if encodingOpts == nil {
		encodingOpts = encoding.NewOptions()
	}

	v = v.SetReaderIteratorAllocate(func(r io.Reader, _ namespace.SchemaDescr) encoding.ReaderIterator {
		intOptimized := m3tsz.DefaultIntOptimizationEnabled
		return m3tsz.NewReaderIterator(r, intOptimized, encodingOpts)
	})

	if c.Proto != nil && c.Proto.Enabled {
		v = v.SetEncodingProto(encodingOpts)
		schemaRegistry := namespace.NewSchemaRegistry(true, nil)
		// Load schema registry from file.
		deployID := "fromfile"
		for nsID, protoConfig := range c.Proto.SchemaRegistry {
			err = namespace.LoadSchemaRegistryFromFile(schemaRegistry, ident.StringID(nsID), deployID, protoConfig.SchemaFilePath, protoConfig.MessageName)
			if err != nil {
				return nil, xerrors.Wrapf(err, "could not load schema registry from file %s for namespace %s", protoConfig.SchemaFilePath, nsID)
			}
		}
		v = v.SetSchemaRegistry(schemaRegistry)
	}

	if c.WriteShardsInitializing != nil {
		v = v.SetWriteShardsInitializing(*c.WriteShardsInitializing)
	}
	if c.ShardsLeavingCountTowardsConsistency != nil {
		v = v.SetShardsLeavingCountTowardsConsistency(*c.ShardsLeavingCountTowardsConsistency)
	}

	// Cast to admin options to apply admin config options.
	opts := v.(AdminOptions)

	if c.WriteTimestampOffset != nil {
		opts = opts.SetWriteTimestampOffset(*c.WriteTimestampOffset)
	}

	if c.FetchSeriesBlocksBatchConcurrency != nil {
		opts = opts.SetFetchSeriesBlocksBatchConcurrency(*c.FetchSeriesBlocksBatchConcurrency)
	}
	if c.FetchSeriesBlocksBatchSize != nil {
		opts = opts.SetFetchSeriesBlocksBatchSize(*c.FetchSeriesBlocksBatchSize)
	}

	// Apply programmatic custom options last.
	for _, opt := range custom {
		opts = opt(opts)
	}

	asyncClusterOpts := NewOptionsForAsyncClusters(opts, asyncTopoInits, asyncClientOverrides)
	return NewAdminClient(opts, asyncClusterOpts...)
}
