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

package topology

import (
	"github.com/m3db/m3/src/cluster/shard"
	"github.com/m3db/m3/src/dbnode/sharding"
	"github.com/m3db/m3/src/x/ident"
	xwatch "github.com/m3db/m3/src/x/watch"
)

type staticMap struct {
	shardSet                 sharding.ShardSet
	hostShardSets            []HostShardSet
	hostShardSetsByID        map[string]HostShardSet
	orderedHosts             []Host
	hostsByShard             [][]Host
	orderedShardHostsByShard [][]orderedShardHost
	replicas                 int
	majority                 int
}

// NewStaticMap creates a new static topology map
func NewStaticMap(opts StaticOptions) Map {
	totalShards := len(opts.ShardSet().AllIDs())
	hostShardSets := opts.HostShardSets()
	topoMap := staticMap{
		shardSet:                 opts.ShardSet(),
		hostShardSets:            hostShardSets,
		hostShardSetsByID:        make(map[string]HostShardSet),
		orderedHosts:             make([]Host, 0, len(hostShardSets)),
		hostsByShard:             make([][]Host, totalShards),
		orderedShardHostsByShard: make([][]orderedShardHost, totalShards),
		replicas:                 opts.Replicas(),
		majority:                 Majority(opts.Replicas()),
	}

	for idx, hostShardSet := range hostShardSets {
		host := hostShardSet.Host()
		topoMap.hostShardSetsByID[host.ID()] = hostShardSet
		topoMap.orderedHosts = append(topoMap.orderedHosts, host)
		for _, shard := range hostShardSet.ShardSet().All() {
			id := shard.ID()
			topoMap.hostsByShard[id] = append(topoMap.hostsByShard[id], host)
			elem := orderedShardHost{
				idx:   idx,
				shard: shard,
				host:  host,
			}
			topoMap.orderedShardHostsByShard[id] =
				append(topoMap.orderedShardHostsByShard[id], elem)
		}
	}

	return &topoMap
}

type orderedShardHost struct {
	idx   int
	shard shard.Shard
	host  Host
}

func (t *staticMap) Hosts() []Host {
	return t.orderedHosts
}

func (t *staticMap) HostShardSets() []HostShardSet {
	return t.hostShardSets
}

func (t *staticMap) LookupHostShardSet(id string) (HostShardSet, bool) {
	value, ok := t.hostShardSetsByID[id]
	return value, ok
}

func (t *staticMap) HostsLen() int {
	return len(t.orderedHosts)
}

func (t *staticMap) ShardSet() sharding.ShardSet {
	return t.shardSet
}

func (t *staticMap) Route(id ident.ID) (uint32, []Host, error) {
	shard := t.shardSet.Lookup(id)
	if int(shard) >= len(t.hostsByShard) {
		return shard, nil, errUnownedShard
	}
	return shard, t.hostsByShard[shard], nil
}

func (t *staticMap) RouteForEach(id ident.ID, forEachFn RouteForEachFn) error {
	return t.RouteShardForEach(t.shardSet.Lookup(id), forEachFn)
}

func (t *staticMap) RouteShard(shard uint32) ([]Host, error) {
	if int(shard) >= len(t.hostsByShard) {
		return nil, errUnownedShard
	}
	return t.hostsByShard[shard], nil
}

func (t *staticMap) RouteShardForEach(shard uint32, forEachFn RouteForEachFn) error {
	if int(shard) >= len(t.orderedShardHostsByShard) {
		return errUnownedShard
	}
	orderedShardHosts := t.orderedShardHostsByShard[shard]
	for _, elem := range orderedShardHosts {
		forEachFn(elem.idx, elem.shard, elem.host)
	}
	return nil
}

func (t *staticMap) Replicas() int {
	return t.replicas
}

func (t *staticMap) MajorityReplicas() int {
	return t.majority
}

type mapWatch struct {
	xwatch.Watch
}

// NewMapWatch creates a new watch on a topology map
// from a generic watch that watches a Map
func NewMapWatch(w xwatch.Watch) MapWatch {
	return &mapWatch{w}
}

func (w *mapWatch) C() <-chan struct{} {
	return w.Watch.C()
}

func (w *mapWatch) Get() Map {
	value := w.Watch.Get()
	if value == nil {
		return nil
	}
	return value.(Map)
}

func (w *mapWatch) Close() {
	w.Watch.Close()
}
