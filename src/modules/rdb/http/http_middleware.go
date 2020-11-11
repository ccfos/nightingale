package http

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/slice"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/rdb/config"
	"github.com/didi/nightingale/src/common/address"
)

func shouldBeLogin() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("username", mustUsername(c))
		c.Next()
	}
}

func shouldBeRoot() gin.HandlerFunc {
	return func(c *gin.Context) {
		username := mustUsername(c)

		user, err := models.UserGet("username=?", username)
		dangerous(err)

		if user.IsRoot != 1 {
			bomb("forbidden")
		}

		c.Set("username", username)
		c.Set("user", user)
		c.Next()
	}
}

func shouldBeService() gin.HandlerFunc {
	return func(c *gin.Context) {
		remoteAddr := c.Request.RemoteAddr
		idx := strings.LastIndex(remoteAddr, ":")
		ip := ""
		if idx > 0 {
			ip = remoteAddr[0:idx]
		}

		if ip == "127.0.0.1" {
			c.Next()
			return
		}


		if ip != "" && slice.ContainsString(address.GetAddresses("rdb"), ip) {
			c.Next()
			return
		}

		token := c.GetHeader("X-Srv-Token")
		if token == "" {
			c.AbortWithError(http.StatusForbidden, fmt.Errorf("X-Srv-Token is blank"))
			return
		}

		if !slice.ContainsString(config.Config.Tokens, token) {
			c.AbortWithError(http.StatusForbidden, fmt.Errorf("X-Srv-Token[%s] invalid", token))
			return
		}

		c.Next()
	}
}

func mustUsername(c *gin.Context) string {
	username := cookieUsername(c)
	if username == "" {
		username = headerUsername(c)
	}

	if username == "" {
		bomb("unauthorized")
	}

	return username
}

func cookieUsername(c *gin.Context) string {
	return models.UsernameByUUID(readCookieUser(c))
}

func headerUsername(c *gin.Context) string {
	token := c.GetHeader("X-User-Token")
	if token == "" {
		return ""
	}

	ut, err := models.UserTokenGet("token=?", token)
	if err != nil {
		logger.Warningf("UserTokenGet[%s] fail: %v", token, err)
		return ""
	}

	if ut == nil {
		return ""
	}

	return ut.Username
}

// ------------

func readCookieUser(c *gin.Context) string {
	uuid, err := c.Cookie(config.Config.HTTP.CookieName)
	if err != nil {
		return ""
	}

	return uuid
}

func writeCookieUser(c *gin.Context, uuid string) {
	c.SetCookie(config.Config.HTTP.CookieName, uuid, 3600*24, "/", config.Config.HTTP.CookieDomain, false, true)
}
