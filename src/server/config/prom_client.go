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

var ReaderClient *PromClient = &PromClient{}

func (pc *PromClient) Set(clusterName string, c prom.API) {
	pc.Lock()
	defer pc.Unlock()
	pc.ClusterName = clusterName
	pc.API = c
}

func (pc *PromClient) Get() (string, prom.API) {
	pc.RLock()
	defer pc.RUnlock()
	return pc.ClusterName, pc.API
}

func (pc *PromClient) GetClusterName() string {
	pc.RLock()
	defer pc.RUnlock()
	return pc.ClusterName
}

func (pc *PromClient) GetCli() prom.API {
	pc.RLock()
	defer pc.RUnlock()
	return pc.API
}

func (pc *PromClient) IsNil() bool {
	if pc == nil {
		return true
	}

	pc.RLock()
	defer pc.RUnlock()

	return pc.API == nil
}

func (pc *PromClient) Reset() {
	pc.Lock()
	defer pc.Unlock()

	pc.ClusterName = ""
	pc.API = nil
}
