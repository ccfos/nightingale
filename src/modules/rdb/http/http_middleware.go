package http

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/slice"

	"github.com/didi/nightingale/src/common/address"
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/rdb/config"
	"github.com/didi/nightingale/src/modules/rdb/session"
)

func shouldStartSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionStart(c)
		c.Next()
		sessionUpdate(c)
	}
}

func shouldBeLogin() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionStart(c)
		username := mustUsername(c)
		c.Set("username", username)
		c.Next()
		sessionUpdate(c)
	}
}

func shouldBeRoot() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionStart(c)
		username := mustUsername(c)

		user, err := models.UserGet("username=?", username)
		dangerous(err)

		if user.IsRoot != 1 {
			bomb("forbidden")
		}

		c.Set("username", username)
		c.Set("user", user)
		c.Next()
		sessionUpdate(c)
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
	username := sessionUsername(c)
	if username == "" {
		username = headerUsername(c)
	}

	if username == "" {
		bomb("unauthorized")
	}

	return username
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

func sessionStart(c *gin.Context) error {
	s, err := session.Start(c.Writer, c.Request)
	if err != nil {
		logger.Warningf("session.Start() err %s", err)
		return err
	}
	c.Request = c.Request.WithContext(session.NewContext(c.Request.Context(), s))
	return nil
}

func sessionUpdate(c *gin.Context) {
	if store, ok := session.FromContext(c.Request.Context()); ok {
		err := store.Update(c.Writer)
		if err != nil {
			logger.Errorf("session update err %s", err)
		}
	}
}

func sessionUsername(c *gin.Context) string {
	s, ok := session.FromContext(c.Request.Context())
	if !ok {
		return ""
	}
	return s.Get("username")
}

func sessionLogin(c *gin.Context, username, remoteAddr, accessToken string) {
	s, ok := session.FromContext(c.Request.Context())
	if !ok {
		logger.Warningf("session.Start() err not found sessionStore")
		return
	}
	if err := s.Set("username", username); err != nil {
		logger.Warningf("session.Set() err %s", err)
		return
	}
	if err := s.Set("remoteAddr", remoteAddr); err != nil {
		logger.Warningf("session.Set() err %s", err)
		return
	}
	if err := s.Set("accessToken", accessToken); err != nil {
		logger.Warningf("session.Set() err %s", err)
		return
	}
}
