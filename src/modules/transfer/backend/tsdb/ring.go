// Copyright 2017 Xiaomi, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tsdb

import (
	"sync"

	"github.com/toolkits/pkg/consistent"
)

type ConsistentHashRing struct {
	sync.RWMutex
	ring *consistent.Consistent
}

func (c *ConsistentHashRing) GetNode(pk string) (string, error) {
	c.RLock()
	defer c.RUnlock()

	return c.ring.Get(pk)
}

func (c *ConsistentHashRing) Set(r *consistent.Consistent) {
	c.Lock()
	defer c.Unlock()
	c.ring = r
}

func (c *ConsistentHashRing) GetRing() *consistent.Consistent {
	c.RLock()
	defer c.RUnlock()

	return c.ring
}

func NewConsistentHashRing(replicas int32, nodes []string) *ConsistentHashRing {
	ret := &ConsistentHashRing{ring: consistent.New()}
	ret.ring.NumberOfReplicas = int(replicas)
	for i := 0; i < len(nodes); i++ {
		ret.ring.Add(nodes[i])
	}
	return ret
}

func RebuildConsistentHashRing(hashRing *ConsistentHashRing, nodes []string, replicas int) {
	r := consistent.New()
	r.NumberOfReplicas = replicas
	for i := 0; i < len(nodes); i++ {
		r.Add(nodes[i])
	}
	hashRing.Set(r)
}
