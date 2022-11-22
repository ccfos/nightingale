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

	labelMap := make(map[string]string)
	for i := 0; i < len(v.Labels); i++ {
		labelMap[v.Labels[i].Name] = v.Labels[i].Value
	}

	filterMap := memsto.LogSampleCache.Get()
	for k, v := range filterMap {
		// 在指标 labels 中找过滤的 label key ，如果找不到，直接返回
		lableValue, exists := labelMap[k]
		if !exists {
			return
		}

		// key 存在，在过滤条件中找指标的 label value，如果找不到，直接返回
		_, exists = v[lableValue]
		if !exists {
			return
		}
	}

	// 每个过滤条件都在 指标的 labels 中找到了
	logger.Debugf("recv sample from:%s sample:%s", remoteAddr, v.String())
}
