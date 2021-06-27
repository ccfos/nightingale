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

// 哈希环是活着的judge实例(因为模块合并，即server实例)组成的
// trans利用哈希环做数据分片计算
// judge利用哈希环做PULL型策略分片计算
var HashRing = NewConsistentHashRing(int32(NodeReplicas), []string{})

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

func RebuildConsistentHashRing(nodes []string) {
	r := consistent.New()
	r.NumberOfReplicas = NodeReplicas
	for i := 0; i < len(nodes); i++ {
		r.Add(nodes[i])
	}

	HashRing.Set(r)

	logger.Infof("hash ring rebuild %+v", r.Members())
}
