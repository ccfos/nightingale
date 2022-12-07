package config

import (
	"sync"

	"github.com/didi/nightingale/v5/src/pkg/prom"
)

type PromClient struct {
	prom.API
	ClusterName string
	sync.RWMutex
}

type PromClientMap struct {
	sync.RWMutex
	Clients map[string]prom.API
}

var ReaderClient *PromClient = &PromClient{}
var ReaderClients *PromClientMap = &PromClientMap{Clients: make(map[string]prom.API)}

func (pc *PromClientMap) Set(clusterName string, c prom.API) {
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
	if pc == nil {
		return true
	}

	pc.RLock()
	defer pc.RUnlock()

	c, exists := pc.Clients[cluster]
	if !exists {
		return true
	}

	return c == nil
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

// func (pc *PromClient) Set(clusterName string, c prom.API) {
// 	pc.Lock()
// 	defer pc.Unlock()
// 	pc.ClusterName = clusterName
// 	pc.API = c
// }

// func (pc *PromClient) Get() (string, prom.API) {
// 	pc.RLock()
// 	defer pc.RUnlock()
// 	return pc.ClusterName, pc.API
// }

// func (pc *PromClient) GetClusterName() string {
// 	pc.RLock()
// 	defer pc.RUnlock()
// 	return pc.ClusterName
// }

// func (pc *PromClient) GetCli() prom.API {
// 	pc.RLock()
// 	defer pc.RUnlock()
// 	return pc.API
// }

// func (pc *PromClient) IsNil() bool {
// 	if pc == nil {
// 		return true
// 	}

// 	pc.RLock()
// 	defer pc.RUnlock()

// 	return pc.API == nil
// }

// func (pc *PromClient) Reset() {
// 	pc.Lock()
// 	defer pc.Unlock()

// 	pc.ClusterName = ""
// 	pc.API = nil
// }
