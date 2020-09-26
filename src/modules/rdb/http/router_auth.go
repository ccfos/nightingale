package http

import (
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/rdb/config"
	"github.com/didi/nightingale/src/modules/rdb/ssoc"
)

type loginForm struct {
	Username   string `json:"username" binding:"required"`
	Password   string `json:"password" binding:"required"`
	IsLDAP     int    `json:"is_ldap"`
	RemoteAddr string `json:"remote_addr"`
}

func (f *loginForm) validate() {
	if str.Dangerous(f.Username) {
		bomb("%s invalid", f.Username)
	}

	if len(f.Username) > 64 {
		bomb("%s too long", f.Username)
	}
}

func login(c *gin.Context) {
	var f loginForm
	bind(c, &f)
	f.validate()

	if f.IsLDAP == 1 {
		dangerous(models.LdapLogin(f.Username, f.Password, c.ClientIP()))
	} else {
		dangerous(models.PassLogin(f.Username, f.Password, c.ClientIP()))
	}

	user, err := models.UserGet("username=?", f.Username)
	dangerous(err)

	writeCookieUser(c, user.UUID)

	renderMessage(c, "")
}

// v1Login called by sso.rdb module
func v1Login(c *gin.Context) {
	var f loginForm
	bind(c, &f)
	f.validate()

	if f.IsLDAP == 1 {
		dangerous(models.LdapLogin(f.Username, f.Password, c.ClientIP()))
	} else {
		dangerous(models.PassLogin(f.Username, f.Password, c.ClientIP()))
	}

	user, err := models.UserGet("username=?", f.Username)
	dangerous(err)

	writeCookieUser(c, user.UUID)

	// TODO: implement remote address access control
	go models.LoginLogNew(f.Username, f.RemoteAddr, "in")

	renderData(c, user, nil)
}

func logout(c *gin.Context) {
	uuid := readCookieUser(c)
	if uuid == "" {
		c.String(200, "logout successfully")
		return
	}

	username := models.UsernameByUUID(uuid)
	if username == "" {
		c.String(200, "logout successfully")
		return
	}

	writeCookieUser(c, "")

	go models.LoginLogNew(username, c.ClientIP(), "out")

	if config.Config.SSO.Enable {
		redirect := queryStr(c, "redirect", "/")
		c.Redirect(302, ssoc.LogoutLocation(redirect))
	} else {
		c.String(200, "logout successfully")
	}
}

func authAuthorize(c *gin.Context) {
	username := cookieUsername(c)
	if username != "" { // alread login
		c.String(200, "hi, "+username)
		return
	}

	redirect := queryStr(c, "redirect", "/")

	if config.Config.SSO.Enable {
		c.Redirect(302, ssoc.Authorize(redirect))
	} else {
		c.Redirect(302, "/login?redirect="+url.QueryEscape(redirect))
	}

}

func authCallback(c *gin.Context) {
	code := queryStr(c, "code")
	state := queryStr(c, "state")

	if code == "" {
		if redirect := queryStr(c, "redirect"); redirect != "" {
			c.Redirect(302, redirect)
			return
		}
	}

	redirect, user, err := ssoc.Callback(code, state)
	dangerous(err)

	writeCookieUser(c, user.UUID)
	c.Redirect(302, redirect)
}
