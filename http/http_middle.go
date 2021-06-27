package http

import (
	"net/http"

	"github.com/didi/nightingale/v5/pkg/ierr"
	"github.com/gin-gonic/gin"
)

func login() gin.HandlerFunc {
	return func(c *gin.Context) {
		username := loginUsername(c)
		c.Set("username", username)
		// 这里调用loginUser主要是为了判断当前用户是否被disable了
		loginUser(c)
		c.Next()
	}
}

func admin() gin.HandlerFunc {
	return func(c *gin.Context) {
		username := loginUsername(c)
		c.Set("username", username)

		user := loginUser(c)
		if user.Role != "Admin" {
			ierr.Bomb(http.StatusForbidden, "forbidden")
		}

		c.Next()
	}
}
