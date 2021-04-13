package http

import (
	"github.com/didi/nightingale/v4/src/common/dataobj"
	statsd "github.com/didi/nightingale/v4/src/common/stats"
	"github.com/didi/nightingale/v4/src/modules/server/rpc"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
)

func PushData(c *gin.Context) {
	if c.Request.ContentLength == 0 {
		renderMessage(c, "blank body")
		return
	}

	recvMetricValues := make([]*dataobj.MetricValue, 0)
	errors.Dangerous(c.ShouldBindJSON(&recvMetricValues))

	errCount, errMsg := rpc.PushData(recvMetricValues)
	statsd.Counter.Set("http.points.in.err", errCount)
	if errMsg != "" {
		renderMessage(c, errMsg)
		return
	}

	renderData(c, "ok", nil)
}
