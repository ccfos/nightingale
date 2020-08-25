package routes

import (
	"fmt"
	"os"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/collector/core"
	"github.com/didi/nightingale/src/modules/collector/log/strategy"
	"github.com/didi/nightingale/src/modules/collector/log/worker"
	"github.com/didi/nightingale/src/modules/collector/stra"
	"github.com/didi/nightingale/src/toolkits/http/render"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
)

func ping(c *gin.Context) {
	c.String(200, "pong")
}

func addr(c *gin.Context) {
	c.String(200, c.Request.RemoteAddr)
}

func pid(c *gin.Context) {
	c.String(200, fmt.Sprintf("%d", os.Getpid()))
}

func pushData(c *gin.Context) {
	if c.Request.ContentLength == 0 {
		render.Message(c, "blank body")
		return
	}

	var recvMetricValues []*dataobj.MetricValue
	errors.Dangerous(c.ShouldBindJSON(&recvMetricValues))

	err := core.Push(recvMetricValues)
	render.Message(c, err)
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

	render.Data(c, resp, nil)
}

func getLogCached(c *gin.Context) {
	render.Data(c, worker.GetCachedAll(), nil)
}
