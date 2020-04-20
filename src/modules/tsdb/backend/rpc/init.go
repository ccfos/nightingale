package rpc

import (
	"github.com/didi/nightingale/src/toolkits/pools"
)

var (
	// 连接池 node_address -> connection_pool
	IndexConnPools *pools.ConnPools
	Config         RpcClientSection
)

type RpcClientSection struct {
	MaxConns    int `yaml:"maxConns"`
	MaxIdle     int `yaml:"maxIdle"`
	ConnTimeout int `yaml:"connTimeout"`
	CallTimeout int `yaml:"callTimeout"`
}

func Init(cfg RpcClientSection, indexes []string) {
	Config = cfg
	IndexConnPools = pools.NewConnPools(cfg.MaxConns, cfg.MaxIdle, cfg.ConnTimeout, cfg.CallTimeout, indexes)
}

func ReNewPools(indexes []string) []string {
	return IndexConnPools.UpdatePools(indexes)
}
