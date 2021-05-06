package http

import (
	"github.com/didi/nightingale/v4/src/modules/server/config"

	"github.com/gin-gonic/gin"
)

func globalOpsGet(c *gin.Context) {
	renderData(c, config.GlobalOps, nil)
}

func localOpsGet(c *gin.Context) {
	renderData(c, config.LocalOps, nil)
}
