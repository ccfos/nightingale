package routes

import (
	"fmt"
	"os"

	"github.com/didi/nightingale/src/modules/index/cache"
	"github.com/didi/nightingale/src/toolkits/http/render"

	"github.com/gin-gonic/gin"
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

func indexTotal(c *gin.Context) {
	endpoints := cache.IndexDB.GetEndpoints()
	var total int
	for _, endpoint := range endpoints {
		metricIndexMap, exists := cache.IndexDB.GetMetricIndexMap(endpoint)
		if !exists || metricIndexMap == nil {
			continue
		}

		metrics := metricIndexMap.GetMetrics()
		for _, metric := range metrics {
			metricIndex, exists := metricIndexMap.GetMetricIndex(metric)
			if !exists || metricIndex == nil {
				continue
			}
			total += metricIndex.CounterMap.Len()
		}
	}
	render.Data(c, total, nil)
}
