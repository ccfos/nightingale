package http

import "github.com/gin-gonic/gin"

func ping(c *gin.Context) {
	c.String(200, "pong")
}
