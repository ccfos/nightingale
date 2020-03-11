package routes

import (
	"os"

	"github.com/didi/nightingale/src/modules/monapi/config"
	"github.com/gin-gonic/gin"
)

func ping(c *gin.Context) {
	c.String(200, "pong")
}

func version(c *gin.Context) {
	c.String(200, "%d", config.Version)
}

func pid(c *gin.Context) {
	c.String(200, "%d", os.Getpid())
}

func addr(c *gin.Context) {
	c.String(200, c.Request.RemoteAddr)
}
