package http

import (
	"fmt"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/monapi/config"
	"github.com/didi/nightingale/src/modules/monapi/tools"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/slice"
)

// GetCookieUser 从cookie中获取username
func GetCookieUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		username := cookieUser(c)
		if username == "" {
			username = headerUser(c)
		}

		if username == "" {
			errors.Bomb("unauthorized")
		}

		c.Set("username", username)
		c.Next()
	}
}

func cookieUser(c *gin.Context) string {
	uuid, err := c.Cookie("ecmc-user")
	if err != nil {
		return ""
	}

	return tools.UsernameByUUID(uuid)
}

func headerUser(c *gin.Context) string {
	token := c.GetHeader("X-User-Token")
	if token == "" {
		return ""
	}

	user, err := getUserByToken(token)
	errors.Dangerous(err)

	if user == nil {
		return ""
	}

	return user.Username
}

const internalToken = "monapi-builtin-token"

// CheckHeaderToken check thirdparty X-Srv-Token
func CheckHeaderToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("X-Srv-Token")
		if token != internalToken && !slice.ContainsString(config.Get().Tokens, token) {
			errors.Bomb("token[%s] invalid", token)
		}
		c.Next()
	}
}

func getUserByToken(token string) (user *models.User, err error) {
	ut, err := models.UserTokenGet("token=?", token)
	if err != nil {
		return
	}

	if ut == nil {
		return user, fmt.Errorf("token not found")
	}

	user, err = models.UserGet("id=?", ut.UserId)
	if err != nil {
		return
	}

	if user == nil {
		return user, fmt.Errorf("user not found")
	}

	return
}
