package backend

import (
	"sync"

	"github.com/toolkits/pkg/consistent"
)

type ConsistentHashRing struct {
	sync.RWMutex
	ring *consistent.Consistent
}

func (this *ConsistentHashRing) GetNode(pk string) (string, error) {
	this.RLock()
	defer this.RUnlock()

	return this.ring.Get(pk)
}

func (this *ConsistentHashRing) Set(r *consistent.Consistent) {
	this.Lock()
	defer this.Unlock()
	this.ring = r
}

func (this *ConsistentHashRing) GetRing() *consistent.Consistent {
	this.RLock()
	defer this.RUnlock()

	return this.ring
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
