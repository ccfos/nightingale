package statsd

import (
	"sync"
)

var (
	// metrics支持的聚合类型
	CommonAggregatorsConst = map[string]bool{
		"c": true, "ce": true, "rpc": true, "r": true, "rt": true,
		"p1": true, "p5": true, "p25": true, "p50": true, "p75": true,
		"p90": true, "p95": true, "p99": true, "rpce": true,
		"max": true, "min": true, "sum": true, "avg": true, "cnt": true,
		"g": true,
	}
	HistogramAggregatorsConst = map[string]bool{
		"p1": true, "p5": true, "p25": true, "p50": true, "p75": true,
		"p90": true, "p95": true, "p99": true,
		"max": true, "min": true, "sum": true, "avg": true, "cnt": true,
	}
	Const_CommonAggregator_Rpc  = "rpc"
	Const_CommonAggregator_RpcE = "rpce"

	// rpc状态码
	RpcOkCodesConst = map[string]bool{"ok": true, "0": true,
		"200": true, "201": true, "203": true}

	// metrics支持的最大tag数
	MaxTagsCntConst = 12

	// ns前缀后缀
	NsPrefixConst = ""
	NsSuffixConst = ""

	// 需要聚合的metric
	MetricToBeSummarized_RpcdisfConst     = "rpcdisf"
	MetricToBeSummarized_RpcdfeConst      = "rpcdfe"
	MetricToBeSummarized_DirpcCallConst   = "rpc_dirpc_call"
	MetricToBeSummarized_DirpcCalledConst = "rpc_dirpc_called"

	// summarize等待collect结束的超时时间
	SummarizeWaitCollectTimeoutMsConst = 2000

	// traceid对应的tagk
	TagTraceId = "traceid"

	// LRU 缓存的大小
	MaxLRUCacheSize = 10000

	// 并包模式下的分隔符
	MergeDelimiter = "&"
	// $value,$statusCode的分隔符, 向前兼容, 使用 ","
	CodeDelimiter = ","
)

var (
	exitLock = &sync.RWMutex{}
	isExited = false
)

func Start() {
	isExited = false

	// 定时从中心拉取配置
	//go MetricAgentConfig{}.UpdateLoop()

	// 开启监控数据上报
	go StatsdReporter{}.Report()
}

func Exit() {
	exitLock.Lock()
	isExited = true
	exitLock.Unlock()
}

func IsExited() bool {
	exitLock.RLock()
	r := isExited
	exitLock.RUnlock()
	return r
}
