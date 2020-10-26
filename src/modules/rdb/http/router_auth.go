package http

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/cache"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/rdb/config"
	"github.com/didi/nightingale/src/modules/rdb/redisc"
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
		c.String(200, "sso does not enable")
	}

}

type authRedirect struct {
	Redirect string `json:"redirect"`
	Msg      string `json:"msg"`
}

func authAuthorizeV2(c *gin.Context) {
	redirect := queryStr(c, "redirect", "/")
	ret := &authRedirect{Redirect: redirect}

	username := cookieUsername(c)
	if username != "" { // alread login
		renderData(c, ret, nil)
		return
	}

	if config.Config.SSO.Enable {
		ret.Redirect = ssoc.Authorize(redirect)
	} else {
		ret.Redirect = "/login"
	}
	renderData(c, ret, nil)
}

func authCallback(c *gin.Context) {
	code := queryStr(c, "code", "")
	state := queryStr(c, "state", "")
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

func authCallbackV2(c *gin.Context) {
	code := queryStr(c, "code", "")
	state := queryStr(c, "state", "")
	redirect := queryStr(c, "redirect", "")

	ret := &authRedirect{Redirect: redirect}
	if code == "" && redirect != "" {
		renderData(c, ret, nil)
		return
	}

	var user *models.User
	var err error
	ret.Redirect, user, err = ssoc.Callback(code, state)
	if err != nil {
		renderData(c, ret, err)
		return
	}

	writeCookieUser(c, user.UUID)
	renderData(c, ret, nil)
}

func authSettings(c *gin.Context) {
	renderData(c, struct {
		Sso bool `json:"sso"`
	}{
		Sso: config.Config.SSO.Enable,
	}, nil)
}

func logoutV2(c *gin.Context) {
	redirect := queryStr(c, "redirect", "")
	ret := &authRedirect{Redirect: redirect}

	uuid := readCookieUser(c)
	if uuid == "" {
		renderData(c, ret, nil)
		return
	}

	username := models.UsernameByUUID(uuid)
	if username == "" {
		renderData(c, ret, nil)
		return
	}

	writeCookieUser(c, "")
	ret.Msg = "logout successfully"

	go models.LoginLogNew(username, c.ClientIP(), "out")

	if config.Config.SSO.Enable {
		if redirect == "" {
			redirect = "/"
		}
		ret.Redirect = ssoc.LogoutLocation(redirect)
	}
	renderData(c, ret, nil)
}

type loginInput struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	Phone      string `json:"phone"`
	Code       string `json:"code"`
	Type       string `json:"type"`
	RemoteAddr string `json:"remote_addr"`
}

// v1Login called by sso.rdb module
func v1Login(c *gin.Context) {
	var f loginInput
	bind(c, &f)

	user, err := func() (*models.User, error) {
		switch strings.ToLower(f.Type) {
		case "ldap":
			err := models.LdapLogin(f.Username, f.Password, c.ClientIP())
			if err != nil {
				return nil, err
			}
			return models.UserGet("username=?", f.Username)
		case "password":
			err := models.PassLogin(f.Username, f.Password, c.ClientIP())
			if err != nil {
				return nil, err
			}
			return models.UserGet("username=?", f.Username)
		case "sms-code":
			return smsCodeVerify(f.Phone, f.Code)
		default:
			return nil, fmt.Errorf("invalid login type %s", f.Type)
		}
	}()

	// TODO: implement remote address access control
	go models.LoginLogNew(f.Username, f.RemoteAddr, "in")

	renderData(c, user, err)
}

type v1SendLoginCodeBySmsInput struct {
	Phone string `json:"phone"`
}

func v1SendLoginCodeBySms(c *gin.Context) {
	var f v1SendLoginCodeBySmsInput
	bind(c, &f)

	msg, err := sendLoginCodeBySms(f.Phone)
	renderData(c, msg, err)
}

func sendLoginCodeBySms(phone string) (string, error) {
	user, _ := models.UserGet("phone=?", phone)
	if user == nil {
		return "", fmt.Errorf("phone %s dose not exist", phone)
	}

	// general a random code and add cache
	code := fmt.Sprintf("%06d", rand.Intn(1000000))
	err := cache.Set(fmt.Sprintf("sms.phone.%s.code.%s", phone, code), user.Username, time.Second*300)
	if err != nil {
		return "", err
	}

	// log.Printf("phone %s code %s", phone, code)

	err := redisc.Write(&dataobj.Message{
		Tos:     []string{phone},
		Content: fmt.Sprintf("sms code: [%s]", code),
	}, config.SMS_QUEUE_NAME)
	return code, err
}

func smsCodeVerify(phone, code string) (*models.User, error) {
	var username string

	// log.Printf("phone %s code %s", phone, code)
	key := fmt.Sprintf("sms.phone.%s.code.%s", phone, code)
	err := cache.Get(key, &username)
	if err != nil {
		return nil, err
	}

	cache.Delete(key)

	return models.UserGet("username=?", username)
}
