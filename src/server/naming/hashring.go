package naming

import (
	"sync"

	"github.com/toolkits/pkg/consistent"
	"github.com/toolkits/pkg/logger"
)

const NodeReplicas = 500

type ClusterHashRingType struct {
	sync.RWMutex
	Rings map[string]*consistent.Consistent
}

// for alert_rule sharding
var ClusterHashRing = ClusterHashRingType{Rings: make(map[string]*consistent.Consistent)}

func NewConsistentHashRing(replicas int32, nodes []string) *consistent.Consistent {
	ret := consistent.New()
	ret.NumberOfReplicas = int(replicas)
	for i := 0; i < len(nodes); i++ {
		ret.Add(nodes[i])
	}
	return ret
}

func RebuildConsistentHashRing(cluster string, nodes []string) {
	r := consistent.New()
	r.NumberOfReplicas = NodeReplicas
	for i := 0; i < len(nodes); i++ {
		r.Add(nodes[i])
	}

	ClusterHashRing.Set(cluster, r)
	logger.Infof("hash ring %s rebuild %+v", cluster, r.Members())
}

func (chr *ClusterHashRingType) GetNode(cluster, pk string) (string, error) {
	chr.RLock()
	defer chr.RUnlock()
	_, exists := chr.Rings[cluster]
	if !exists {
		chr.Rings[cluster] = NewConsistentHashRing(int32(NodeReplicas), []string{})
	}

	return chr.Rings[cluster].Get(pk)
}

func (chr *ClusterHashRingType) IsHit(cluster string, pk string, currentNode string) bool {
	node, err := chr.GetNode(cluster, pk)
	if err != nil {
		logger.Debugf("cluster:%s pk:%s failed to get node from hashring:%v", cluster, pk, err)
		return false
	}
	return node == currentNode
}

func (chr *ClusterHashRingType) Set(cluster string, r *consistent.Consistent) {
	chr.RLock()
	defer chr.RUnlock()
	chr.Rings[cluster] = r
}
