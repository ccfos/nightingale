package http

import (
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/v5/config"
	"github.com/didi/nightingale/v5/models"
)

type loginForm struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func loginPost(c *gin.Context) {
	var f loginForm
	bind(c, &f)

	user, err1 := models.PassLogin(f.Username, f.Password)
	if err1 == nil {
		if user.Status == 1 {
			renderMessage(c, "User disabled")
			return
		}
		session := sessions.Default(c)
		session.Set("username", f.Username)
		session.Save()
		renderData(c, user, nil)
		return
	}

	// password login fail, try ldap
	if config.Config.LDAP.Enable {
		user, err2 := models.LdapLogin(f.Username, f.Password)
		if err2 == nil {
			if user.Status == 1 {
				renderMessage(c, "User disabled")
				return
			}
			session := sessions.Default(c)
			session.Set("username", f.Username)
			session.Save()
			renderData(c, user, nil)
			return
		}
	}

	// password and ldap both fail
	renderMessage(c, err1)
}

func logoutGet(c *gin.Context) {
	session := sessions.Default(c)
	session.Set("username", "")
	session.Save()
	renderMessage(c, nil)
}

func canDoOpByName(c *gin.Context) {
	user, err := models.UserGetByUsername(queryStr(c, "name"))
	dangerous(err)

	if user == nil {
		renderData(c, false, err)
		return
	}

	can, err := user.CanDo(queryStr(c, "op"))
	renderData(c, can, err)
}

func canDoOpByToken(c *gin.Context) {
	userToken, err := models.UserTokenGet("token=?", queryStr(c, "token"))
	dangerous(err)

	if userToken == nil {
		renderData(c, false, err)
		return
	}

	user, err := models.UserGetByUsername(userToken.Username)
	dangerous(err)

	if user == nil {
		renderData(c, false, err)
		return
	}

	can, err := user.CanDo(queryStr(c, "op"))
	renderData(c, can, err)
}
