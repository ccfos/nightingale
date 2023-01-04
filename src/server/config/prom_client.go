package config

import (
	"strings"
	"sync"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/pkg/prom"
)

type PromClientMap struct {
	sync.RWMutex
	Clients map[string]prom.API
}

var ReaderClients = &PromClientMap{Clients: make(map[string]prom.API)}

func (pc *PromClientMap) Set(clusterName string, c prom.API) {
	if c == nil {
		return
	}
	pc.Lock()
	defer pc.Unlock()
	pc.Clients[clusterName] = c
}

func (pc *PromClientMap) GetClusterNames() []string {
	pc.RLock()
	defer pc.RUnlock()
	var clusterNames []string
	for k := range pc.Clients {
		clusterNames = append(clusterNames, k)
	}

	return clusterNames
}

func (pc *PromClientMap) GetCli(cluster string) prom.API {
	pc.RLock()
	defer pc.RUnlock()
	c := pc.Clients[cluster]
	return c
}

func (pc *PromClientMap) IsNil(cluster string) bool {
	pc.RLock()
	defer pc.RUnlock()

	c, exists := pc.Clients[cluster]
	if !exists {
		return true
	}

	return c == nil
}

// Hit 根据当前有效的cluster和规则的cluster配置计算有效的cluster列表
func (pc *PromClientMap) Hit(cluster string) []string {
	pc.RLock()
	defer pc.RUnlock()
	clusters := make([]string, 0, len(pc.Clients))
	if cluster == models.ClusterAll {
		for c := range pc.Clients {
			clusters = append(clusters, c)
		}
		return clusters
	}

	ruleClusters := strings.Fields(cluster)
	for c := range pc.Clients {
		for _, rc := range ruleClusters {
			if rc == c {
				clusters = append(clusters, c)
				continue
			}
		}
	}
	return clusters
}

func (pc *PromClientMap) Reset() {
	pc.Lock()
	defer pc.Unlock()

	pc.Clients = make(map[string]prom.API)
}

func (pc *PromClientMap) Del(cluster string) {
	pc.Lock()
	defer pc.Unlock()
	delete(pc.Clients, cluster)
}
