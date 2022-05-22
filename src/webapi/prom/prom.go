package prom

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/didi/nightingale/v5/src/webapi/config"
)

type ClusterType struct {
	Opts      config.ClusterOptions
	Transport *http.Transport
}

type ClustersType struct {
	datas map[string]ClusterType
	mutex *sync.RWMutex
}

func (cs *ClustersType) Put(name string, cluster ClusterType) {
	cs.mutex.Lock()
	cs.datas[name] = cluster
	cs.mutex.Unlock()
}

func (cs *ClustersType) Get(name string) (ClusterType, bool) {
	cs.mutex.RLock()
	defer cs.mutex.RUnlock()

	c, has := cs.datas[name]
	return c, has
}

var Clusters = ClustersType{
	datas: make(map[string]ClusterType),
	mutex: new(sync.RWMutex),
}

func Init() error {
	if config.C.ClustersFrom != "" && config.C.ClustersFrom != "config" {
		return nil
	}

	opts := config.C.Clusters

	for i := 0; i < len(opts); i++ {
		cluster := ClusterType{
			Opts: opts[i],
			Transport: &http.Transport{
				// TLSClientConfig: tlsConfig,
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout: time.Duration(opts[i].DialTimeout) * time.Millisecond,
				}).DialContext,
				ResponseHeaderTimeout: time.Duration(opts[i].Timeout) * time.Millisecond,
				MaxIdleConnsPerHost:   opts[i].MaxIdleConnsPerHost,
			},
		}
		Clusters.Put(opts[i].Name, cluster)
	}

	return nil
}
