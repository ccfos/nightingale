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

package shard

import (
	"math"

	"github.com/m3db/m3/src/cluster/generated/proto/placementpb"
)

const (
	// UnInitializedValue is the unintialized value used in proto.
	UnInitializedValue int64 = 0

	// DefaultShardCutoverNanos is the default shard-level cutover nanos
	// if the value is not set in the proto message.
	DefaultShardCutoverNanos int64 = 0

	// DefaultShardCutoffNanos is the default shard-level cutoff nanos
	// if the value is not set in the proto message.
	DefaultShardCutoffNanos int64 = math.MaxInt64
)

// State represents the state of a shard.
type State int

const (
	// Unknown represents a shard in unknown state.
	Unknown State = iota
	// Initializing represents a shard newly assigned to an instance.
	Initializing
	// Available represents a shard bootstraped and ready to serve.
	Available
	// Leaving represents a shard that is intending to be removed.
	Leaving
)

// A Shard represents a piece of data owned by the service.
type Shard interface {
	// ID returns the ID of the shard.
	ID() uint32

	// CutoverNanos returns when shard traffic is cut over.
	CutoverNanos() int64

	// SetCutoverNanos sets when shard traffic is cut over.
	SetCutoverNanos(value int64) Shard

	// CutoffNanos returns when shard traffic is cut off.
	CutoffNanos() int64

	// SetCutoffNanos sets when shard traffic is cut off.
	SetCutoffNanos(value int64) Shard

	// State returns the state of the shard.
	State() State

	// SetState sets the state of the shard.
	SetState(s State) Shard

	// Source returns the source of the shard.
	SourceID() string

	// SetSource sets the source of the shard.
	SetSourceID(sourceID string) Shard

	// Equals returns whether the shard equals to another shard.
	Equals(s Shard) bool

	// Proto returns the proto representation for the shard.
	Proto() (*placementpb.Shard, error)

	// Clone returns a clone of the Shard.
	Clone() Shard
}

// Shards is a collection of shards owned by one ServiceInstance.
type Shards interface {
	// All returns the shards sorted ascending.
	All() []Shard

	// AllIDs returns the shard IDs sorted ascending.
	AllIDs() []uint32

	// NumShards returns the number of the shards.
	NumShards() int

	// ShardsForState returns the shards in a certain state.
	ShardsForState(state State) []Shard

	// NumShardsForState returns the number of shards in a certain state.
	NumShardsForState(state State) int

	// Add adds a shard.
	Add(shard Shard)

	// Remove removes a shard.
	Remove(shard uint32)

	// Contains checks if a shard exists.
	Contains(shard uint32) bool

	// Shard returns the shard for the id.
	Shard(id uint32) (Shard, bool)

	// Equals returns whether the shards equals to another shards.
	Equals(s Shards) bool

	// String returns the string representation of the shards.
	String() string

	// Proto returns the proto representation for the shards.
	Proto() ([]*placementpb.Shard, error)

	// Clone returns a clone of the Shards.
	Clone() Shards
}
