package query

import (
	"github.com/didi/nightingale/src/toolkits/address"
	"github.com/didi/nightingale/src/toolkits/pools"
)

var (
	TransferConnPools *pools.ConnPools

	connTimeout int32
	callTimeout int32

	Config SeriesQuerySection
)

type SeriesQuerySection struct {
	MaxConn          int    `json:"maxConn"`     //
	MaxIdle          int    `json:"maxIdle"`     //
	ConnTimeout      int    `json:"connTimeout"` // 连接超时
	CallTimeout      int    `json:"callTimeout"` // 请求超时
	IndexMod         string `json:"indexMod"`
	IndexPath        string `json:"indexPath"`
	IndexCallTimeout int    `json:"indexCallTimeout"` // 请求超时
}

func Init(cfg SeriesQuerySection) {
	Config = cfg
	TransferConnPools = pools.NewConnPools(
		Config.MaxConn, Config.MaxIdle, Config.ConnTimeout, Config.CallTimeout, address.GetRPCAddresses("transfer"),
	)

	go GetIndexLoop()
}
