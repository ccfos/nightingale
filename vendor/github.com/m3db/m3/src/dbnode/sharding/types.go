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

package sharding

import (
	"github.com/m3db/m3/src/cluster/shard"
	"github.com/m3db/m3/src/x/ident"
)

// HashGen generates HashFn based on the length of shards.
type HashGen func(length int) HashFn

// HashFn is a sharding hash function.
type HashFn func(id ident.ID) uint32

// ShardSet contains a sharding function and a set of shards, this interface
// allows for potentially out of order shard sets.
type ShardSet interface {
	// All returns a slice to the shards in this set.
	All() []shard.Shard

	// AllIDs returns a slice to the shard IDs in this set.
	AllIDs() []uint32

	// Lookup will return a shard for a given identifier.
	Lookup(id ident.ID) uint32

	// LookupStateByID returns the state of the shard with a given ID.
	LookupStateByID(shardID uint32) (shard.State, error)

	// Min returns the smallest shard owned by this shard set.
	Min() uint32

	// Max returns the largest shard owned by this shard set.
	Max() uint32

	// HashFn returns the sharding hash function.
	HashFn() HashFn
}
