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

package block

import (
	"sync"
	"time"

	"github.com/m3db/m3/src/dbnode/namespace"
	"github.com/m3db/m3/src/dbnode/sharding"
	"github.com/m3db/m3/src/dbnode/x/xio"
	"github.com/m3db/m3/src/x/context"
	"github.com/m3db/m3/src/x/ident"
)

// NewDatabaseBlockRetrieverFn is a method for constructing
// new database block retrievers
type NewDatabaseBlockRetrieverFn func(
	md namespace.Metadata,
	shardSet sharding.ShardSet,
) (DatabaseBlockRetriever, error)

// NewDatabaseBlockRetrieverManager creates a new manager
// for constructing and providing existing database block retrievers
func NewDatabaseBlockRetrieverManager(
	newDatabaseBlockRetrieverFn NewDatabaseBlockRetrieverFn,
) DatabaseBlockRetrieverManager {
	return &blockRetrieverManager{
		newRetrieverFn: newDatabaseBlockRetrieverFn,
		retrievers:     newRetrieverMap(retrieverMapOptions{}),
	}
}

type blockRetrieverManager struct {
	sync.RWMutex
	newRetrieverFn NewDatabaseBlockRetrieverFn
	retrievers     *retrieverMap
}

func (m *blockRetrieverManager) Retriever(
	md namespace.Metadata,
	shardSet sharding.ShardSet,
) (DatabaseBlockRetriever, error) {
	m.RLock()
	retriever, ok := m.retrievers.Get(md.ID())
	m.RUnlock()
	if ok {
		return retriever, nil
	}

	m.Lock()
	defer m.Unlock()

	retriever, ok = m.retrievers.Get(md.ID())
	if ok {
		return retriever, nil
	}

	var err error
	retriever, err = m.newRetrieverFn(md, shardSet)
	if err != nil {
		return nil, err
	}

	m.retrievers.Set(md.ID(), retriever)
	return retriever, nil
}

type shardBlockRetriever struct {
	DatabaseBlockRetriever
	shard uint32
}

// NewDatabaseShardBlockRetriever creates a new shard database
// block retriever given an existing database block retriever
func NewDatabaseShardBlockRetriever(
	shard uint32,
	r DatabaseBlockRetriever,
) DatabaseShardBlockRetriever {
	return &shardBlockRetriever{
		DatabaseBlockRetriever: r,
		shard:                  shard,
	}
}

func (r *shardBlockRetriever) Stream(
	ctx context.Context,
	id ident.ID,
	blockStart time.Time,
	onRetrieve OnRetrieveBlock,
	nsCtx namespace.Context,
) (xio.BlockReader, error) {
	return r.DatabaseBlockRetriever.Stream(ctx, r.shard, id,
		blockStart, onRetrieve, nsCtx)
}

type shardBlockRetrieverManager struct {
	sync.RWMutex
	retriever       DatabaseBlockRetriever
	shardRetrievers map[uint32]DatabaseShardBlockRetriever
}

// NewDatabaseShardBlockRetrieverManager creates and holds shard block
// retrievers binding shards to an existing retriever.
func NewDatabaseShardBlockRetrieverManager(
	r DatabaseBlockRetriever,
) DatabaseShardBlockRetrieverManager {
	return &shardBlockRetrieverManager{
		retriever:       r,
		shardRetrievers: make(map[uint32]DatabaseShardBlockRetriever),
	}
}

func (m *shardBlockRetrieverManager) ShardRetriever(
	shard uint32,
) DatabaseShardBlockRetriever {
	m.RLock()
	retriever, ok := m.shardRetrievers[shard]
	m.RUnlock()

	if ok {
		return retriever
	}

	m.Lock()
	defer m.Unlock()

	retriever, ok = m.shardRetrievers[shard]
	if ok {
		return retriever
	}

	retriever = NewDatabaseShardBlockRetriever(shard, m.retriever)
	m.shardRetrievers[shard] = retriever
	return retriever
}
