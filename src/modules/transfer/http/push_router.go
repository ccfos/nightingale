package http

import (
	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/transfer/rpc"
	"github.com/didi/nightingale/src/toolkits/http/render"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
)

func PushData(c *gin.Context) {
	if c.Request.ContentLength == 0 {
		render.Message(c, "blank body")
		return
	}

	recvMetricValues := make([]*dataobj.MetricValue, 0)
	errors.Dangerous(c.ShouldBindJSON(&recvMetricValues))

	errCount, errMsg := rpc.PushData(recvMetricValues)
	stats.Counter.Set("http.points.in.err", errCount)
	if errMsg != "" {
		render.Message(c, errMsg)
		return
	}

	render.Data(c, "ok", nil)
}
