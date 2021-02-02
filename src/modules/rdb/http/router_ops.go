package http

import (
	"github.com/didi/nightingale/src/modules/rdb/cache"
	"github.com/didi/nightingale/src/modules/rdb/config"
	"github.com/gin-gonic/gin"
)

func globalOpsGet(c *gin.Context) {
	renderData(c, config.GlobalOps, nil)
}

func localOpsGet(c *gin.Context) {
	renderData(c, config.LocalOps, nil)
}

func counterGet(c *gin.Context) {
	renderData(c, cache.GetCounter(), nil)
}
