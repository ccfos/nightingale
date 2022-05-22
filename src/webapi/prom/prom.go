package prom

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/didi/nightingale/v5/src/webapi/config"
)

type ClusterType struct {
	Opts      config.ClusterOptions
	Transport *http.Transport
}

type ClustersType struct {
	datas map[string]*ClusterType
	mutex *sync.RWMutex
}

func (cs *ClustersType) Put(name string, cluster *ClusterType) {
	cs.mutex.Lock()
	cs.datas[name] = cluster
	cs.mutex.Unlock()
}

func (cs *ClustersType) Get(name string) (*ClusterType, bool) {
	cf := strings.ToLower(strings.TrimSpace(config.C.ClustersFrom))

	cs.mutex.RLock()
	c, has := cs.datas[name]
	cs.mutex.RUnlock()
	if has {
		return c, true
	}

	if cf == "" || cf == "config" {
		return nil, false
	}

	// read from api
	if cf == "api" {
		return cs.GetFromAPI(name)
	}

	return nil, false
}

func (cs *ClustersType) GetFromAPI(name string) (*ClusterType, bool) {
	// get from api, parse body
	// 1. not found? return nil, false
	// 2. found? new ClusterType, put, return
	opt := config.ClusterOptions{
		Name:                name,
		Prom:                "",
		BasicAuthUser:       "",
		BasicAuthPass:       "",
		Timeout:             60000,
		DialTimeout:         5000,
		MaxIdleConnsPerHost: 32,
	}

	cluster := &ClusterType{
		Opts: opt,
		Transport: &http.Transport{
			// TLSClientConfig: tlsConfig,
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout: time.Duration(opt.DialTimeout) * time.Millisecond,
			}).DialContext,
			ResponseHeaderTimeout: time.Duration(opt.Timeout) * time.Millisecond,
			MaxIdleConnsPerHost:   opt.MaxIdleConnsPerHost,
		},
	}

	cs.Put(opt.Name, cluster)
	return cluster, true
}

var Clusters = ClustersType{
	datas: make(map[string]*ClusterType),
	mutex: new(sync.RWMutex),
}

func Init() error {
	if config.C.ClustersFrom != "" && config.C.ClustersFrom != "config" {
		return nil
	}

	opts := config.C.Clusters

	for i := 0; i < len(opts); i++ {
		cluster := &ClusterType{
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
