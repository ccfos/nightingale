package routes

import (
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/src/model"
)

type loginForm struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	IsLDAP   int    `json:"is_ldap"`
}

func login(c *gin.Context) {
	var f loginForm
	errors.Dangerous(c.ShouldBind(&f))

	if str.Dangerous(f.Username) {
		errors.Bomb("%s invalid", f.Username)
	}

	if len(f.Username) > 64 {
		errors.Bomb("%s too long", f.Username)
	}

	user := f.Username
	pass := f.Password

	if f.IsLDAP == 1 {
		errors.Dangerous(model.LdapLogin(user, pass))
	} else {
		errors.Dangerous(model.PassLogin(user, pass))
	}

	session := sessions.Default(c)
	session.Set("username", user)
	session.Save()
	renderMessage(c, "")
}

func logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Set("username", "")
	session.Save()
	renderMessage(c, "")
}
