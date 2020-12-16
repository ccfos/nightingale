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

package namespace

import (
	"time"

	"github.com/m3db/m3/src/cluster/client"
	"github.com/m3db/m3/src/dbnode/retention"
	xclose "github.com/m3db/m3/src/x/close"
	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/m3/src/x/instrument"

	"github.com/gogo/protobuf/proto"
)

// Options controls namespace behavior
type Options interface {
	// Validate validates the options
	Validate() error

	// Equal returns true if the provide value is equal to this one
	Equal(value Options) bool

	// SetBootstrapEnabled sets whether this namespace requires bootstrapping
	SetBootstrapEnabled(value bool) Options

	// BootstrapEnabled returns whether this namespace requires bootstrapping
	BootstrapEnabled() bool

	// SetFlushEnabled sets whether the in-memory data for this namespace needs to be flushed
	SetFlushEnabled(value bool) Options

	// FlushEnabled returns whether the in-memory data for this namespace needs to be flushed
	FlushEnabled() bool

	// SetSnapshotEnabled sets whether the in-memory data for this namespace should be snapshotted regularly
	SetSnapshotEnabled(value bool) Options

	// SnapshotEnabled returns whether the in-memory data for this namespace should be snapshotted regularly
	SnapshotEnabled() bool

	// SetWritesToCommitLog sets whether writes for series in this namespace need to go to commit log
	SetWritesToCommitLog(value bool) Options

	// WritesToCommitLog returns whether writes for series in this namespace need to go to commit log
	WritesToCommitLog() bool

	// SetCleanupEnabled sets whether this namespace requires cleaning up fileset/snapshot files
	SetCleanupEnabled(value bool) Options

	// CleanupEnabled returns whether this namespace requires cleaning up fileset/snapshot files
	CleanupEnabled() bool

	// SetRepairEnabled sets whether the data for this namespace needs to be repaired
	SetRepairEnabled(value bool) Options

	// RepairEnabled returns whether the data for this namespace needs to be repaired
	RepairEnabled() bool

	// SetColdWritesEnabled sets whether cold writes are enabled for this namespace.
	SetColdWritesEnabled(value bool) Options

	// ColdWritesEnabled returns whether cold writes are enabled for this namespace.
	ColdWritesEnabled() bool

	// SetCacheBlocksOnRetrieve sets whether to cache blocks from this namespace when retrieved.
	// If global CacheBlocksOnRetrieve option in config.BlockRetrievePolicy is set to false,
	// then that will override any namespace-specific CacheBlocksOnRetrieve options set to true.
	SetCacheBlocksOnRetrieve(value bool) Options

	// CacheBlocksOnRetrieve returns whether to cache blocks from this namespace when retrieved.
	CacheBlocksOnRetrieve() bool

	// SetRetentionOptions sets the retention options for this namespace
	SetRetentionOptions(value retention.Options) Options

	// RetentionOptions returns the retention options for this namespace
	RetentionOptions() retention.Options

	// SetIndexOptions sets the IndexOptions.
	SetIndexOptions(value IndexOptions) Options

	// IndexOptions returns the IndexOptions.
	IndexOptions() IndexOptions

	// SetSchemaHistory sets the schema registry for this namespace.
	SetSchemaHistory(value SchemaHistory) Options

	// SchemaHistory returns the schema registry for this namespace.
	SchemaHistory() SchemaHistory

	// SetRuntimeOptions sets the RuntimeOptions.
	SetRuntimeOptions(value RuntimeOptions) Options

	// RuntimeOptions returns the RuntimeOptions.
	RuntimeOptions() RuntimeOptions

	// SetExtendedOptions sets the ExtendedOptions.
	SetExtendedOptions(value ExtendedOptions) Options

	// ExtendedOptions returns the dynamically typed ExtendedOptions (requires type check on access).
	ExtendedOptions() ExtendedOptions

	// SetAggregationOptions sets the aggregation-related options for this namespace.
	SetAggregationOptions(value AggregationOptions) Options

	// AggregationOptions returns the aggregation-related options for this namespace.
	AggregationOptions() AggregationOptions
}

// IndexOptions controls the indexing options for a namespace.
type IndexOptions interface {
	// Equal returns true if the provide value is equal to this one.
	Equal(value IndexOptions) bool

	// SetEnabled sets whether indexing is enabled.
	SetEnabled(value bool) IndexOptions

	// Enabled returns whether indexing is enabled.
	Enabled() bool

	// SetBlockSize returns the block size.
	SetBlockSize(value time.Duration) IndexOptions

	// BlockSize returns the block size.
	BlockSize() time.Duration
}

// SchemaDescr describes the schema for a complex type value.
type SchemaDescr interface {
	// DeployId returns the deploy id of the schema.
	DeployId() string
	// PrevDeployId returns the previous deploy id of the schema.
	PrevDeployId() string
	// Get returns the message descriptor for the schema.
	Get() MessageDescriptor
	// String returns the compact text of the message descriptor.
	String() string
	// Equal returns true if the provided value is equal to this one.
	Equal(SchemaDescr) bool
}

// SchemaHistory represents schema history for a namespace.
type SchemaHistory interface {
	// Equal returns true if the provided value is equal to this one.
	Equal(SchemaHistory) bool

	// Extends returns true iif the provided value has a lineage to this one.
	Extends(SchemaHistory) bool

	// Get gets the schema descriptor for the specified deploy id.
	Get(id string) (SchemaDescr, bool)

	// GetLatest gets the latest version of schema descriptor.
	GetLatest() (SchemaDescr, bool)
}

// SchemaListener listens for updates to schema registry for a namespace.
type SchemaListener interface {
	// SetSchemaHistory is called when the listener is registered
	// and when any updates occurred passing the new schema history.
	SetSchemaHistory(value SchemaHistory)
}

// SchemaRegistry represents the schema registry for a database.
// It is where dynamic schema updates are delivered into,
// and where schema is retrieved from at series read and write path.
type SchemaRegistry interface {
	// GetLatestSchema gets the latest schema for the namespace.
	// If proto is not enabled, nil, nil is returned
	GetLatestSchema(id ident.ID) (SchemaDescr, error)

	// GetSchema gets the latest schema for the namespace.
	// If proto is not enabled, nil, nil is returned
	GetSchema(id ident.ID, schemaID string) (SchemaDescr, error)

	// SetSchemaHistory sets the schema history for the namespace.
	// If proto is not enabled, nil is returned
	SetSchemaHistory(id ident.ID, history SchemaHistory) error

	// RegisterListener registers a schema listener for the namespace.
	// If proto is not enabled, nil, nil is returned
	RegisterListener(id ident.ID, listener SchemaListener) (xclose.SimpleCloser, error)

	// Close closes all the listeners.
	Close()
}

// Metadata represents namespace metadata information
type Metadata interface {
	// Equal returns true if the provide value is equal to this one
	Equal(value Metadata) bool

	// ID is the ID of the namespace
	ID() ident.ID

	// Options is the namespace options
	Options() Options
}

// Map is mapping from known namespaces' ID to their Metadata
type Map interface {
	// Equal returns true if the provide value is equal to this one
	Equal(value Map) bool

	// Get gets the metadata for the provided namespace
	Get(ident.ID) (Metadata, error)

	// IDs returns the ID of known namespaces
	IDs() []ident.ID

	// Metadatas returns the metadata of known namespaces
	Metadatas() []Metadata
}

// Watch is a watch on a namespace Map
type Watch interface {
	// C is the notification channel for when a value becomes available
	C() <-chan struct{}

	// Get the current namespace map
	Get() Map

	// Close closes the watch
	Close() error
}

// Registry is an un-changing container for a Map
type Registry interface {
	// Watch for the Registry changes
	Watch() (Watch, error)

	// Close closes the registry
	Close() error
}

// Initializer can init new instances of namespace registries
type Initializer interface {
	// Init will return a new Registry
	Init() (Registry, error)
}

// DynamicOptions is a set of options for dynamic namespace registry
type DynamicOptions interface {
	// Validate validates the options
	Validate() error

	// SetInstrumentOptions sets the instrumentation options
	SetInstrumentOptions(value instrument.Options) DynamicOptions

	// InstrumentOptions returns the instrumentation options
	InstrumentOptions() instrument.Options

	// SetConfigServiceClient sets the client of ConfigService
	SetConfigServiceClient(c client.Client) DynamicOptions

	// ConfigServiceClient returns the client of ConfigService
	ConfigServiceClient() client.Client

	// SetNamespaceRegistryKey sets the kv-store key used for the
	// NamespaceRegistry
	SetNamespaceRegistryKey(k string) DynamicOptions

	// NamespaceRegistryKey returns the kv-store key used for the
	// NamespaceRegistry
	NamespaceRegistryKey() string

	// SetForceColdWritesEnabled sets whether or not to force enable cold writes
	// for all ns.
	SetForceColdWritesEnabled(enabled bool) DynamicOptions

	// ForceColdWritesEnabled returns whether or not to force enable cold writes
	// for all ns.
	ForceColdWritesEnabled() bool
}

// NamespaceWatch watches for namespace updates.
type NamespaceWatch interface {
	// Start starts the namespace watch.
	Start() error

	// Stop stops the namespace watch.
	Stop() error

	// close stops the watch, and releases any held resources.
	Close() error
}

// NamespaceUpdater is a namespace updater function.
type NamespaceUpdater func(Map) error

// ExtendedOptions is the type for dynamically typed options.
type ExtendedOptions interface {
	// ToProto converts ExtendedOptions to the corresponding protobuf message.
	ToProto() (msg proto.Message, typeURLPrefix string)

	// Validate validates the ExtendedOptions.
	Validate() error
}

// AggregationOptions is a set of options for aggregating data
// within the namespace.
type AggregationOptions interface {
	// Equal returns true if the provided value is equal to this one.
	Equal(value AggregationOptions) bool

	// SetAggregations sets the aggregations for this namespace.
	SetAggregations(value []Aggregation) AggregationOptions

	// Aggregations returns the aggregations for this namespace.
	Aggregations() []Aggregation
}

// Aggregation describes data points within the namespace.
type Aggregation struct {
	// Aggregated is true if data points are aggregated, false otherwise.
	Aggregated bool

	// Attributes specifies how to aggregate data when aggregated is set to true.
	// This field is ignored when aggregated is false.
	Attributes AggregatedAttributes
}

// AggregationAttributes are attributes specifying how data points should be aggregated.
type AggregatedAttributes struct {
	// Resolution is the time range to aggregate data across.
	Resolution time.Duration

	// DownsampleOptions stores options around how data points are downsampled.
	DownsampleOptions DownsampleOptions
}

// DownsampleOptions is a set of options related to downsampling data.
type DownsampleOptions struct {
	// All indicates whether to send data points to this namespace.
	// If set to false, this namespace will not receive data points. In this
	// case, data will need to be sent to the namespace via another mechanism
	// (e.g. rollup/recording rules).
	All bool
}
