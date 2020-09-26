package scache

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
	return
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

type NodeMap struct {
	sync.RWMutex
	data map[string]string
}

func NewNodeMap() NodeMap {
	nm := NodeMap{
		data: make(map[string]string),
	}
	return nm
}

func (n *NodeMap) GetInstanceBy(node string) (string, bool) {
	n.RLock()
	defer n.RUnlock()
	v, exists := n.data[node]
	return v, exists
}

func (n *NodeMap) GetNodeBy(instance string) (string, bool) {
	n.RLock()
	defer n.RUnlock()
	for node, v := range n.data {
		if instance == v {
			return node, true
		}
	}
	return "", false
}

func (n *NodeMap) GetNodes() []string {
	n.RLock()
	defer n.RUnlock()
	nodes := []string{}
	for node, _ := range n.data {
		nodes = append(nodes, node)
	}
	return nodes
}

func (n *NodeMap) Set(nodeMap map[string]string) {
	n.Lock()
	defer n.Unlock()
	n.data = make(map[string]string)
	for node, ip := range nodeMap {
		n.data[node] = ip
	}
	return
}

func (n *NodeMap) Len() int {
	n.RLock()
	defer n.RUnlock()
	return len(n.data)
}
