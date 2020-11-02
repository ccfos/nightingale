package http

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/rdb/config"
	"github.com/didi/nightingale/src/modules/rdb/redisc"
	"github.com/didi/nightingale/src/modules/rdb/ssoc"
)

var (
	loginCodeSmsTpl   *template.Template
	loginCodeEmailTpl *template.Template
)

func init() {
	var err error
	filename := path.Join(file.SelfDir(), "etc", "login-code-sms.tpl")
	loginCodeSmsTpl, err = template.ParseFiles(filename)
	if err != nil {
		log.Fatalf("open %s err: %s", filename, err)
	}

	filename = path.Join(file.SelfDir(), "etc", "login-code-email.tpl")
	loginCodeEmailTpl, err = template.ParseFiles(filename)
	if err != nil {
		log.Fatalf("open %s err: %s", filename, err)
	}
}

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
	Email      string `json:"email"`
	Code       string `json:"code"`
	Type       string `json:"type"`
	RemoteAddr string `json:"remote_addr"`
}

const (
	LOGIN_T_SMS      = "sms-code"
	LOGIN_T_EMAIL    = "email-code"
	LOGIN_T_RST      = "rst-code"
	LOGIN_T_PWD      = "password"
	LOGIN_T_LDAP     = "ldap"
	LOGIN_EXPIRES_IN = 300
)

// v1Login called by sso.rdb module
func v1Login(c *gin.Context) {
	var f loginInput
	bind(c, &f)

	user, err := func() (*models.User, error) {
		switch strings.ToLower(f.Type) {
		case LOGIN_T_LDAP:
			err := models.LdapLogin(f.Username, f.Password, c.ClientIP())
			if err != nil {
				return nil, err
			}
			return models.UserGet("username=?", f.Username)
		case LOGIN_T_PWD:
			err := models.PassLogin(f.Username, f.Password, c.ClientIP())
			if err != nil {
				return nil, err
			}
			return models.UserGet("username=?", f.Username)
		case LOGIN_T_SMS:
			return smsCodeVerify(f.Phone, f.Code)
		case LOGIN_T_EMAIL:
			return emailCodeVerify(f.Email, f.Code)
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

	msg, err := func() (string, error) {
		if !config.Config.Redis.Enable {
			return "", fmt.Errorf("sms sender is disabled")
		}
		phone := f.Phone
		user, _ := models.UserGet("phone=?", phone)
		if user == nil {
			return "", fmt.Errorf("phone %s dose not exist", phone)
		}

		// general a random code and add cache
		code := fmt.Sprintf("%06d", rand.Intn(1000000))

		loginCode := &models.LoginCode{
			Username:  user.Username,
			Code:      code,
			LoginType: LOGIN_T_SMS,
			CreatedAt: time.Now().Unix(),
		}

		if err := loginCode.Save(); err != nil {
			return "", err
		}

		var buf bytes.Buffer
		if err := loginCodeSmsTpl.Execute(&buf, loginCode); err != nil {
			return "", err
		}

		if err := redisc.Write(&dataobj.Message{
			Tos:     []string{phone},
			Content: buf.String(),
		}, config.SMS_QUEUE_NAME); err != nil {
			return "", err
		}

		// log.Printf("[sms -> %s] %s", phone, buf.String())

		// TODO: remove code from msg
		return fmt.Sprintf("[debug] msg: %s", buf.String()), nil

	}()
	renderData(c, msg, err)
}

func smsCodeVerify(phone, code string) (*models.User, error) {
	user, _ := models.UserGet("phone=?", phone)
	if user == nil {
		return nil, fmt.Errorf("phone %s dose not exist", phone)
	}

	lc, err := models.LoginCodeGet("username=? and code=? and login_type=?", user.Username, code, LOGIN_T_SMS)
	if err != nil {
		return nil, fmt.Errorf("invalid code", phone)
	}

	if time.Now().Unix()-lc.CreatedAt > LOGIN_EXPIRES_IN {
		return nil, fmt.Errorf("the code has expired", phone)
	}

	lc.Del()

	return user, nil
}

type v1SendLoginCodeByEmailInput struct {
	Email string `json:"email"`
}

func v1SendLoginCodeByEmail(c *gin.Context) {
	var f v1SendLoginCodeByEmailInput
	bind(c, &f)

	msg, err := func() (string, error) {
		if !config.Config.Redis.Enable {
			return "", fmt.Errorf("mail sender is disabled")
		}
		email := f.Email
		user, _ := models.UserGet("email=?", email)
		if user == nil {
			return "", fmt.Errorf("email %s dose not exist", email)
		}

		// general a random code and add cache
		code := fmt.Sprintf("%06d", rand.Intn(1000000))

		loginCode := &models.LoginCode{
			Username:  user.Username,
			Code:      code,
			LoginType: LOGIN_T_EMAIL,
			CreatedAt: time.Now().Unix(),
		}

		if err := loginCode.Save(); err != nil {
			return "", err
		}

		var buf bytes.Buffer
		if err := loginCodeEmailTpl.Execute(&buf, loginCode); err != nil {
			return "", err
		}

		err := redisc.Write(&dataobj.Message{
			Tos:     []string{email},
			Content: buf.String(),
		}, config.SMS_QUEUE_NAME)

		// log.Printf("[email -> %s] %s", email, buf.String())

		// TODO: remove code from msg
		return fmt.Sprintf("[debug] msg: %s", buf.String()), err

	}()
	renderData(c, msg, err)
}

func emailCodeVerify(email, code string) (*models.User, error) {
	user, _ := models.UserGet("email=?", email)
	if user == nil {
		return nil, fmt.Errorf("email %s dose not exist", email)
	}

	lc, err := models.LoginCodeGet("username=? and code=? and login_type=?", user.Username, code, LOGIN_T_EMAIL)
	if err != nil {
		return nil, fmt.Errorf("invalid code", email)
	}

	if time.Now().Unix()-lc.CreatedAt > LOGIN_EXPIRES_IN {
		return nil, fmt.Errorf("the code has expired", email)
	}

	lc.Del()

	return user, nil
}

type sendRstCodeBySmsInput struct {
	Phone string `json:"phone"`
}

func sendRstCodeBySms(c *gin.Context) {
	var f sendRstCodeBySmsInput
	bind(c, &f)

	msg, err := func() (string, error) {
		if !config.Config.Redis.Enable {
			return "", fmt.Errorf("sms sender is disabled")
		}
		phone := f.Phone
		user, _ := models.UserGet("phone=?", phone)
		if user == nil {
			return "", fmt.Errorf("phone %s dose not exist", phone)
		}

		// general a random code and add cache
		code := fmt.Sprintf("%06d", rand.Intn(1000000))

		loginCode := &models.LoginCode{
			Username:  user.Username,
			Code:      code,
			LoginType: LOGIN_T_RST,
			CreatedAt: time.Now().Unix(),
		}

		if err := loginCode.Save(); err != nil {
			return "", err
		}

		var buf bytes.Buffer
		if err := loginCodeSmsTpl.Execute(&buf, loginCode); err != nil {
			return "", err
		}

		if err := redisc.Write(&dataobj.Message{
			Tos:     []string{phone},
			Content: buf.String(),
		}, config.SMS_QUEUE_NAME); err != nil {
			return "", err
		}

		// log.Printf("[sms -> %s] %s", phone, buf.String())

		// TODO: remove code from msg
		return fmt.Sprintf("[debug] msg: %s", buf.String()), nil

	}()
	renderData(c, msg, err)
}

type rstPasswordInput struct {
	Phone    string `json:"phone"`
	Code     string `json:"code"`
	Password string `json:"password"`
}

func rstPassword(c *gin.Context) {
	var in loginInput
	bind(c, &in)

	err := func() error {
		user, _ := models.UserGet("phone=?", in.Phone)
		if user == nil {
			return fmt.Errorf("phone %s dose not exist", in.Phone)
		}

		lc, err := models.LoginCodeGet("username=? and code=? and login_type=?",
			user.Username, in.Code, LOGIN_T_RST)
		if err != nil {
			return fmt.Errorf("invalid code", in.Phone)
		}

		if time.Now().Unix()-lc.CreatedAt > LOGIN_EXPIRES_IN {
			return fmt.Errorf("the code has expired", in.Phone)
		}

		// update password
		if user.Password, err = models.CryptoPass(in.Password); err != nil {
			return err
		}

		if err = user.Update("password"); err != nil {
			return err
		}

		lc.Del()

		return nil
	}()

	if err != nil {
		renderData(c, nil, err)
	} else {
		renderData(c, "reset successfully", nil)
	}
}
