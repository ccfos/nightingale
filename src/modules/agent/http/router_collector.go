package http

import (
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/agent/core"
	"github.com/didi/nightingale/src/modules/agent/log/strategy"
	"github.com/didi/nightingale/src/modules/agent/log/worker"
	"github.com/didi/nightingale/src/modules/agent/stra"
)

func pushData(c *gin.Context) {
	if c.Request.ContentLength == 0 {
		renderMessage(c, "blank body")
		return
	}

	var recvMetricValues []*dataobj.MetricValue
	errors.Dangerous(c.ShouldBindJSON(&recvMetricValues))

	err := core.Push(recvMetricValues)
	renderMessage(c, err)
}

func getStrategy(c *gin.Context) {
	var resp []interface{}

	port := stra.GetPortCollects()
	for _, s := range port {
		resp = append(resp, s)
	}

	proc := stra.GetProcCollects()
	for _, s := range proc {
		resp = append(resp, s)
	}

	logStras := strategy.GetListAll()
	for _, s := range logStras {
		resp = append(resp, s)
	}

	renderData(c, resp, nil)
}

func getLogCached(c *gin.Context) {
	renderData(c, worker.GetCachedAll(), nil)
}
