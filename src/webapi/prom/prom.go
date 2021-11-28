package prom

import (
	"net"
	"net/http"
	"time"
)

type Options struct {
	Name string
	Prom string

	BasicAuthUser string
	BasicAuthPass string

	Timeout               int64
	DialTimeout           int64
	TLSHandshakeTimeout   int64
	ExpectContinueTimeout int64
	IdleConnTimeout       int64
	KeepAlive             int64

	MaxConnsPerHost     int
	MaxIdleConns        int
	MaxIdleConnsPerHost int
}

type ClusterType struct {
	Opts      Options
	Transport *http.Transport
}

type ClustersType struct {
	M map[string]ClusterType
}

func NewClusters() ClustersType {
	return ClustersType{
		M: make(map[string]ClusterType),
	}
}

func (cs *ClustersType) Put(name string, cluster ClusterType) {
	cs.M[name] = cluster
}

func (cs *ClustersType) Get(name string) (ClusterType, bool) {
	c, has := cs.M[name]
	return c, has
}

var Clusters = NewClusters()

func Init(opts []Options) error {
	for i := 0; i < len(opts); i++ {
		cluster := ClusterType{
			Opts: opts[i],
			Transport: &http.Transport{
				// TLSClientConfig: tlsConfig,
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   time.Duration(opts[i].DialTimeout) * time.Millisecond,
					KeepAlive: time.Duration(opts[i].KeepAlive) * time.Millisecond,
				}).DialContext,
				ResponseHeaderTimeout: time.Duration(opts[i].Timeout) * time.Millisecond,
				TLSHandshakeTimeout:   time.Duration(opts[i].TLSHandshakeTimeout) * time.Millisecond,
				ExpectContinueTimeout: time.Duration(opts[i].ExpectContinueTimeout) * time.Millisecond,
				MaxConnsPerHost:       opts[i].MaxConnsPerHost,
				MaxIdleConns:          opts[i].MaxIdleConns,
				MaxIdleConnsPerHost:   opts[i].MaxIdleConnsPerHost,
				IdleConnTimeout:       time.Duration(opts[i].IdleConnTimeout) * time.Millisecond,
			},
		}
		Clusters.Put(opts[i].Name, cluster)
	}

	return nil
}
