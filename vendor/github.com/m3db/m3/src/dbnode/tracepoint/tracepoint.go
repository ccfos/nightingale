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

package tracepoint

// The tracepoint package is used to store operation names for tracing throughout dbnode.
// The naming convention is as follows:

// `packageName.objectName.method`

// If there isn't an object, use `packageName.method`.

const (
	// FetchTagged is the operation name for the tchannelthrift FetchTagged path.
	FetchTagged = "tchannelthrift/node.service.FetchTagged"

	// Query is the operation name for the tchannelthrift Query path.
	Query = "tchannelthrift/node.service.Query"

	// FetchReadEncoded is the operation name for the tchannelthrift FetchReadEncoded path.
	FetchReadEncoded = "tchannelthrift/node.service.FetchReadEncoded"

	// FetchReadResults is the operation name for the tchannelthrift FetchReadResults path.
	FetchReadResults = "tchannelthrift/node.service.FetchReadResults"

	// FetchReadSingleResult is the operation name for the tchannelthrift FetchReadSingleResult path.
	FetchReadSingleResult = "tchannelthrift/node.service.FetchReadSingleResult"

	// FetchReadSegment is the operation name for the tchannelthrift FetchReadSegment path.
	FetchReadSegment = "tchannelthrift/node.service.FetchReadSegment"

	// AggregateTiles is the operation name for the tchannelthrift AggregateTiles path.
	AggregateTiles = "tchannelthrift/node.service.AggregateTiles"

	// DBQueryIDs is the operation name for the db QueryIDs path.
	DBQueryIDs = "storage.db.QueryIDs"

	// DBAggregateQuery is the operation name for the db AggregateQuery path.
	DBAggregateQuery = "storage.db.AggregateQuery"

	// DBReadEncoded is the operation name for the db ReadEncoded path.
	DBReadEncoded = "storage.db.ReadEncoded"

	// DBFetchBlocks is the operation name for the db FetchBlocks path.
	DBFetchBlocks = "storage.db.FetchBlocks"

	// DBFetchBlocksMetadataV2 is the operation name for the db FetchBlocksMetadataV2 path.
	DBFetchBlocksMetadataV2 = "storage.db.FetchBlocksMetadataV2"

	// DBWriteBatch is the operation name for the db WriteBatch path.
	DBWriteBatch = "storage.db.WriteBatch"

	// DBAggregateTiles is the operation name for the db AggregateTiles path.
	DBAggregateTiles = "storage.db.AggregateTiles"

	// NSQueryIDs is the operation name for the dbNamespace QueryIDs path.
	NSQueryIDs = "storage.dbNamespace.QueryIDs"

	// NSPrepareBootstrap is the operation name for the dbNamespace PrepareBootstrap path.
	NSPrepareBootstrap = "storage.dbNamespace.PrepareBootstrap"

	// NSBootstrap is the operation name for the dbNamespace Bootstrap path.
	NSBootstrap = "storage.dbNamespace.Bootstrap"

	// ShardPrepareBootstrap is the operation name for the dbShard PrepareBootstrap path.
	ShardPrepareBootstrap = "storage.dbShard.PrepareBootstrap"

	// ShardBootstrap is the operation name for the dbShard Bootstrap path.
	ShardBootstrap = "storage.dbShard.Bootstrap"

	// NSIdxQuery is the operation name for the nsIndex Query path.
	NSIdxQuery = "storage.nsIndex.Query"

	// NSIdxAggregateQuery is the operation name for the nsIndex AggregateQuery path.
	NSIdxAggregateQuery = "storage.nsIndex.AggregateQuery"

	// NSIdxQueryHelper is the operation name for the nsIndex query path.
	NSIdxQueryHelper = "storage.nsIndex.query"

	// NSIdxBlockQuery is the operation name for the nsIndex block query path.
	NSIdxBlockQuery = "storage.nsIndex.blockQuery"

	// NSIdxBlockAggregateQuery is the operation name for the nsIndex block aggregate query path.
	NSIdxBlockAggregateQuery = "storage.nsIndex.blockAggregateQuery"

	// BlockQuery is the operation name for the index block query path.
	BlockQuery = "storage/index.block.Query"

	// BlockAggregate is the operation name for the index block aggregate path.
	BlockAggregate = "storage/index.block.Aggregate"

	// BootstrapProcessRun is the operation name for the bootstrap process Run path.
	BootstrapProcessRun = "bootstrap.bootstrapProcess.Run"

	// BootstrapperUninitializedSourceRead is the operation for the uninitializedTopologySource Read path.
	BootstrapperUninitializedSourceRead = "bootstrapper.uninitialized.uninitializedTopologySource.Read"

	// BootstrapperCommitLogSourceRead is the operation for the commit log Read path.
	BootstrapperCommitLogSourceRead = "bootstrapper.commitlog.commitLogSource.Read"

	// BootstrapperPeersSourceRead is the operation for the peers Read path.
	BootstrapperPeersSourceRead = "bootstrapper.peers.peersSource.Read"

	// BootstrapperFilesystemSourceRead is the operation for the filesystem Read path.
	BootstrapperFilesystemSourceRead = "bootstrapper.fs.filesystemSource.Read"

	// BootstrapperFilesystemSourceMigrator is the operation for filesystem migrator path.
	BootstrapperFilesystemSourceMigrator = "bootstrapper.fs.filesystemSource.Migrator"
)
