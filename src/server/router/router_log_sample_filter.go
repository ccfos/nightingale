package router

import (
	"github.com/didi/nightingale/v5/src/server/memsto"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/prometheus/prompb"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
)

func logSampleFilterAdd(c *gin.Context) {
	var f map[string][]string
	ginx.BindJSON(c, &f)

	memsto.LogSampleCache.Set(f)
	c.JSON(200, "ok")
}

func logSampleFilterGet(c *gin.Context) {
	c.JSON(200, memsto.LogSampleCache.Get())
}

func logSampleFilterDel(c *gin.Context) {
	memsto.LogSampleCache.Clean()
	c.JSON(200, "ok")
}

func LogSample(remoteAddr string, v *prompb.TimeSeries) {
	if memsto.LogSampleCache.Len() == 0 {
		return
	}

	for j := 0; j < len(v.Labels); j++ {
		if exists := memsto.LogSampleCache.Exists(v.Labels[j].Name, v.Labels[j].Value); exists {
			logger.Debugf("recv sample from:%s sample:%s", remoteAddr, v.String())
			break
		}
	}
}
