package http

import (
	"fmt"
	"os"

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
