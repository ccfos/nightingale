package http

import (
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/slice"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/ams/config"
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
		token := c.GetHeader("X-Srv-Token")
		if token == "" {
			bomb("X-Srv-Token is blank")
		}
		if !slice.ContainsString(config.Config.Tokens, token) {
			bomb("X-Srv-Token[%s] invalid", token)
		}
		c.Next()
	}
}

func mustUsername(c *gin.Context) string {
	username := sessionUsername(c)
	if username == "" {
		username = headerUsername(c)
	}

	if username == "" {
		bomb("unauthorized")
	}

	return username
}

func sessionUsername(c *gin.Context) string {
	sess, err := models.SessionGetWithCache(readSessionId(c))
	if err != nil {
		return ""
	}
	return sess.Username
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

func readSessionId(c *gin.Context) string {
	sid, err := c.Cookie(config.Config.HTTP.CookieName)
	if err != nil {
		return ""
	}
	return sid
}
