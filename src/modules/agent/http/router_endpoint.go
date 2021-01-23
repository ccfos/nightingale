package http

import (
	"github.com/didi/nightingale/src/modules/agent/config"
	"github.com/gin-gonic/gin"
)

func endpoint(c *gin.Context) {
	c.String(200, config.Endpoint)
}
