package routes

import (
	"fmt"
	"time"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/transfer/backend"
	"github.com/didi/nightingale/src/toolkits/http/render"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

func PushData(c *gin.Context) {
	now := time.Now().Unix()
	if c.Request.ContentLength == 0 {
		render.Message(c, "blank body")
		return
	}

	recvMetricValues := make([]*dataobj.MetricValue, 0)
	metricValues := make([]*dataobj.MetricValue, 0)
	errors.Dangerous(c.ShouldBindJSON(&recvMetricValues))

	var msg string
	for _, v := range recvMetricValues {
		logger.Debug("->recv: ", v)
		stats.Counter.Set("points.in", 1)

		err := v.CheckValidity(now)
		if err != nil {
			stats.Counter.Set("points.in.err", 1)
			msg += fmt.Sprintf("recv metric %v err:%v\n", v, err)
			logger.Warningf(msg)
			continue
		}
		metricValues = append(metricValues, v)
	}

	// send to judge
	backend.Push2JudgeQueue(metricValues)

	// send to push endpoints
	pushEndpoints, err := backend.GetPushEndpoints()
	if err != nil {
		logger.Errorf("could not find pushendpoint")
		render.Data(c, "error", err)
		return
	} else {
		for _, pushendpoint := range pushEndpoints {
			pushendpoint.Push2Queue(metricValues)
		}
	}

	if msg != "" {
		render.Message(c, msg)
		return
	}

	render.Data(c, "ok", nil)
}
