package naming

import (
	"sync"

	"github.com/toolkits/pkg/consistent"
	"github.com/toolkits/pkg/logger"
)

const NodeReplicas = 500

type ConsistentHashRing struct {
	sync.RWMutex
	ring *consistent.Consistent
}

type ClusterHashRingType struct {
	sync.RWMutex
	Rings map[string]*ConsistentHashRing
}

// for alert_rule sharding
var ClusterHashRing = ClusterHashRingType{Rings: make(map[string]*ConsistentHashRing)}

func (chr *ConsistentHashRing) GetNode(pk string) (string, error) {
	chr.RLock()
	defer chr.RUnlock()

	return chr.ring.Get(pk)
}

func (chr *ConsistentHashRing) Set(r *consistent.Consistent) {
	chr.Lock()
	defer chr.Unlock()
	chr.ring = r
}

func (chr *ConsistentHashRing) GetRing() *consistent.Consistent {
	chr.RLock()
	defer chr.RUnlock()

	return chr.ring
}

func NewConsistentHashRing(replicas int32, nodes []string) *ConsistentHashRing {
	ret := &ConsistentHashRing{ring: consistent.New()}
	ret.ring.NumberOfReplicas = int(replicas)
	for i := 0; i < len(nodes); i++ {
		ret.ring.Add(nodes[i])
	}
	return ret
}

func RebuildConsistentHashRing(cluster string, nodes []string) {
	r := consistent.New()
	r.NumberOfReplicas = NodeReplicas
	for i := 0; i < len(nodes); i++ {
		r.Add(nodes[i])
	}

	ClusterHashRing.GetRing(cluster).Set(nodes)
	logger.Infof("hash ring %s rebuild %+v", cluster, r.Members())
}

func (chr *ClusterHashRingType) GetRing(cluster string) *consistent.Consistent {
	chr.RLock()
	defer chr.RUnlock()
	_, exists := chr.Rings[cluster]
	if !exists {
		chr.Rings[cluster] = NewConsistentHashRing(int32(NodeReplicas), []string{})
	}

	return chr.Rings[cluster].GetRing()
}

func (chr *ClusterHashRingType) GetNode(cluster, pk string) (string, error) {
	chr.RLock()
	defer chr.RUnlock()
	_, exists := chr.Rings[cluster]
	if !exists {
		chr.Rings[cluster] = NewConsistentHashRing(int32(NodeReplicas), []string{})
	}

	return chr.Rings[cluster].GetNode(pk)
}
