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
	"errors"
	"math"

	"github.com/m3db/m3/src/cluster/shard"
	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/stackmurmur3/v2"
)

var (
	// ErrDuplicateShards returned when shard set is empty
	ErrDuplicateShards = errors.New("duplicate shards")

	// ErrInvalidShardID is returned on an invalid shard ID
	ErrInvalidShardID = errors.New("no shard with given ID")
)

type shardSet struct {
	shards   []shard.Shard
	ids      []uint32
	shardMap map[uint32]shard.Shard
	fn       HashFn
}

// NewShardSet creates a new sharding scheme with a set of shards
func NewShardSet(shards []shard.Shard, fn HashFn) (ShardSet, error) {
	if err := validateShards(shards); err != nil {
		return nil, err
	}
	return newValidatedShardSet(shards, fn), nil
}

// NewEmptyShardSet creates a new sharding scheme with an empty set of shards
func NewEmptyShardSet(fn HashFn) ShardSet {
	return newValidatedShardSet(nil, fn)
}

func newValidatedShardSet(shards []shard.Shard, fn HashFn) ShardSet {
	ids := make([]uint32, len(shards))
	shardMap := make(map[uint32]shard.Shard, len(shards))
	for i, shard := range shards {
		ids[i] = shard.ID()
		shardMap[shard.ID()] = shard
	}
	return &shardSet{
		shards:   shards,
		ids:      ids,
		shardMap: shardMap,
		fn:       fn,
	}
}

func (s *shardSet) Lookup(identifier ident.ID) uint32 {
	return s.fn(identifier)
}

func (s *shardSet) LookupStateByID(shardID uint32) (shard.State, error) {
	hostShard, ok := s.shardMap[shardID]
	if !ok {
		return shard.State(0), ErrInvalidShardID
	}
	return hostShard.State(), nil
}

func (s *shardSet) All() []shard.Shard {
	return s.shards[:]
}

func (s *shardSet) AllIDs() []uint32 {
	return s.ids[:]
}

func (s *shardSet) Min() uint32 {
	min := uint32(math.MaxUint32)
	for _, shard := range s.ids {
		if shard < min {
			min = shard
		}
	}
	return min
}

func (s *shardSet) Max() uint32 {
	max := uint32(0)
	for _, shard := range s.ids {
		if shard > max {
			max = shard
		}
	}
	return max
}

func (s *shardSet) HashFn() HashFn {
	return s.fn
}

// NewShards returns a new slice of shards with a specified state
func NewShards(ids []uint32, state shard.State) []shard.Shard {
	shards := make([]shard.Shard, len(ids))
	for i, id := range ids {
		shards[i] = shard.NewShard(id).SetState(state)
	}
	return shards
}

// IDs returns a new slice of shard IDs for a set of shards
func IDs(shards []shard.Shard) []uint32 {
	ids := make([]uint32, len(shards))
	for i := range ids {
		ids[i] = shards[i].ID()
	}
	return ids
}

func validateShards(shards []shard.Shard) error {
	uniqueShards := make(map[uint32]struct{}, len(shards))
	for _, s := range shards {
		if _, exist := uniqueShards[s.ID()]; exist {
			return ErrDuplicateShards
		}
		uniqueShards[s.ID()] = struct{}{}
	}
	return nil
}

// DefaultHashFn generates a HashFn based on murmur32
func DefaultHashFn(length int) HashFn {
	return NewHashFn(length, 0)
}

// NewHashGenWithSeed generates a HashFnGen based on murmur32 with a given seed
func NewHashGenWithSeed(seed uint32) HashGen {
	return func(length int) HashFn {
		return NewHashFn(length, seed)
	}
}

// NewHashFn generates a HashFN based on murmur32 with a given seed
func NewHashFn(length int, seed uint32) HashFn {
	return func(id ident.ID) uint32 {
		return murmur3.SeedSum32(seed, id.Bytes()) % uint32(length)
	}
}
