package rpc

import (
	"github.com/toolkits/pkg/pool"
)

var (
	// 连接池 node_address -> connection_pool
	IndexConnPools *ConnPools = &ConnPools{M: make(map[string]*pool.ConnPool)}
	Config         RpcClientSection
)

type RpcClientSection struct {
	MaxConns    int `yaml:"maxConns"`
	MaxIdle     int `yaml:"maxIdle"`
	ConnTimeout int `yaml:"connTimeout"`
	CallTimeout int `yaml:"callTimeout"`
}

func Init(cfg RpcClientSection, indexs []string) {
	Config = cfg
	IndexConnPools = CreateConnPools(cfg.MaxConns, cfg.MaxIdle,
		cfg.ConnTimeout, cfg.CallTimeout, indexs)
}

func ReNewPools(indexs []string) []string {
	return IndexConnPools.UpdatePools(indexs)
}
