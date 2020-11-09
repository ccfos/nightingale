package http

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mojocn/base64Captcha"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/rdb/config"
	"github.com/didi/nightingale/src/modules/rdb/redisc"
	"github.com/didi/nightingale/src/modules/rdb/ssoc"
)

var (
	loginCodeSmsTpl     *template.Template
	loginCodeEmailTpl   *template.Template
	errUnsupportCaptcha = errors.New("unsupported captcha")
	errInvalidAnswer    = errors.New("Invalid captcha answer")

	// TODO: set false
	debug = true

	// https://captcha.mojotv.cn
	captchaDirver = base64Captcha.DriverString{
		Height:          30,
		Width:           120,
		ShowLineOptions: 0,
		Length:          4,
		Source:          "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789",
		//ShowLineOptions: 14,
	}
)

func getConfigFile(name, ext string) (string, error) {
	if p := path.Join(path.Join(file.SelfDir(), "etc", name+".local."+ext)); file.IsExist(p) {
		return p, nil
	}
	if p := path.Join(path.Join(file.SelfDir(), "etc", name+"."+ext)); file.IsExist(p) {
		return p, nil
	} else {
		return "", fmt.Errorf("file %s not found", p)
	}

}

func init() {
	filename, err := getConfigFile("login-code-sms", "tpl")
	if err != nil {
		log.Fatal(err)
	}

	loginCodeSmsTpl, err = template.ParseFiles(filename)
	if err != nil {
		log.Fatalf("open %s err: %s", filename, err)
	}

	filename, err = getConfigFile("login-code-email", "tpl")
	if err != nil {
		log.Fatal(err)
	}
	loginCodeEmailTpl, err = template.ParseFiles(filename)
	if err != nil {
		log.Fatalf("open %s err: %s", filename, err)
	}
}

func login(c *gin.Context) {
	var f loginInput
	bind(c, &f)
	f.validate()

	if config.Config.Captcha {
		c, err := models.CaptchaGet("captcha_id=?", f.CaptchaId)
		dangerous(err)
		if strings.ToLower(c.Answer) != strings.ToLower(f.Answer) {
			dangerous(errInvalidAnswer)
		}
	}

	user, err := authLogin(f)
	dangerous(err)

	writeCookieUser(c, user.UUID)

	renderMessage(c, "")

	go models.LoginLogNew(user.Username, c.ClientIP(), "in")
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

	if config.Config.SSO.Enable {
		redirect := queryStr(c, "redirect", "/")
		c.Redirect(302, ssoc.LogoutLocation(redirect))
	} else {
		c.String(200, "logout successfully")
	}

	go models.LoginLogNew(username, c.ClientIP(), "out")
}

type authRedirect struct {
	Redirect string `json:"redirect"`
	Msg      string `json:"msg"`
}

func authAuthorizeV2(c *gin.Context) {
	redirect := queryStr(c, "redirect", "/")
	log.Printf("---> redirect %s", redirect)
	ret := &authRedirect{Redirect: redirect}

	username := cookieUsername(c)
	if username != "" { // alread login
		renderData(c, ret, nil)
		return
	}

	var err error
	if config.Config.SSO.Enable {
		ret.Redirect, err = ssoc.Authorize(redirect)
	} else {
		ret.Redirect = "/login"
	}
	renderData(c, ret, err)
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

	if config.Config.SSO.Enable {
		if redirect == "" {
			redirect = "/"
		}
		ret.Redirect = ssoc.LogoutLocation(redirect)
	}

	renderData(c, ret, nil)

	go models.LoginLogNew(username, c.ClientIP(), "out")
}

type loginInput struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	Phone      string `json:"phone"`
	Email      string `json:"email"`
	Code       string `json:"code"`
	CaptchaId  string `json:"captcha_id"`
	Answer     string `json:"answer" description:"captcha answer"`
	Type       string `json:"type" description:"sms-code|email-code|password|ldap"`
	RemoteAddr string `json:"remote_addr" description:"use for server account(v1)"`
	IsLDAP     int    `json:"is_ldap" description:"deprecated"`
}

func (f *loginInput) validate() {
	if f.IsLDAP == 1 {
		f.Type = models.LOGIN_T_LDAP
	}
	if f.Type == "" {
		f.Type = models.LOGIN_T_PWD
	}
	if f.Type == models.LOGIN_T_PWD {
		if str.Dangerous(f.Username) {
			bomb("%s invalid", f.Username)
		}
		if len(f.Username) > 64 {
			bomb("%s too long", f.Username)
		}
	}
}

// v1Login called by sso.rdb module
func v1Login(c *gin.Context) {
	var f loginInput
	bind(c, &f)

	user, err := authLogin(f)
	renderData(c, *user, err)

	go models.LoginLogNew(user.Username, f.RemoteAddr, "in")
}

// authLogin called by /v1/rdb/login, /api/rdb/auth/login
func authLogin(in loginInput) (user *models.User, err error) {
	switch strings.ToLower(in.Type) {
	case models.LOGIN_T_LDAP:
		return models.LdapLogin(in.Username, in.Password)
	case models.LOGIN_T_PWD:
		return models.PassLogin(in.Username, in.Password)
	case models.LOGIN_T_SMS:
		return models.SmsCodeLogin(in.Phone, in.Code)
	case models.LOGIN_T_EMAIL:
		return models.EmailCodeLogin(in.Email, in.Code)
	default:
		return nil, fmt.Errorf("invalid login type %s", in.Type)
	}
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
			LoginType: models.LOGIN_T_SMS,
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

		if debug {
			return fmt.Sprintf("[debug]: %s", buf.String()), nil
		}

		return "successed", nil

	}()
	renderData(c, msg, err)
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
			LoginType: models.LOGIN_T_EMAIL,
			CreatedAt: time.Now().Unix(),
		}

		if err := loginCode.Save(); err != nil {
			return "", err
		}

		var buf bytes.Buffer
		if err := loginCodeEmailTpl.Execute(&buf, loginCode); err != nil {
			return "", err
		}

		if err := redisc.Write(&dataobj.Message{
			Tos:     []string{email},
			Content: buf.String(),
		}, config.SMS_QUEUE_NAME); err != nil {
			return "", err
		}

		if debug {
			return fmt.Sprintf("[debug]: %s", buf.String()), nil
		}
		return "successed", nil
	}()
	renderData(c, msg, err)
}

type sendRstCodeBySmsInput struct {
	Username string `json:"username"`
	Phone    string `json:"phone"`
}

func sendRstCodeBySms(c *gin.Context) {
	var f sendRstCodeBySmsInput
	bind(c, &f)

	msg, err := func() (string, error) {
		if !config.Config.Redis.Enable {
			return "", fmt.Errorf("sms sender is disabled")
		}
		phone := f.Phone
		user, _ := models.UserGet("username=? and phone=?", f.Username, phone)
		if user == nil {
			return "", fmt.Errorf("user %s phone %s dose not exist", f.Username, phone)
		}

		// general a random code and add cache
		code := fmt.Sprintf("%06d", rand.Intn(1000000))

		loginCode := &models.LoginCode{
			Username:  user.Username,
			Code:      code,
			LoginType: models.LOGIN_T_RST,
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

		if debug {
			return fmt.Sprintf("[debug] msg: %s", buf.String()), nil
		}

		return "successed", nil

	}()
	renderData(c, msg, err)
}

type rstPasswordInput struct {
	Username string `json:"username"`
	Phone    string `json:"phone"`
	Code     string `json:"code"`
	Password string `json:"password"`
	Type     string `json:"type"`
}

func rstPassword(c *gin.Context) {
	var in rstPasswordInput
	bind(c, &in)

	err := func() error {
		user, _ := models.UserGet("username=? and phone=?", in.Username, in.Phone)
		if user == nil {
			return fmt.Errorf("user's phone  not exist")
		}

		lc, err := models.LoginCodeGet("username=? and code=? and login_type=?",
			user.Username, in.Code, models.LOGIN_T_RST)
		if err != nil {
			return fmt.Errorf("invalid code")
		}

		if time.Now().Unix()-lc.CreatedAt > models.LOGIN_EXPIRES_IN {
			return fmt.Errorf("the code has expired")
		}

		if in.Type == "verify-code" {
			return nil
		}
		defer lc.Del()

		// update password
		if user.Password, err = models.CryptoPass(in.Password); err != nil {
			return err
		}

		if err = checkPassword(in.Password); err != nil {
			return err
		}

		if err = user.Update("password"); err != nil {
			return err
		}

		return nil
	}()

	if err != nil {
		renderData(c, nil, err)
	} else {
		renderData(c, "reset successfully", nil)
	}
}

func captchaGet(c *gin.Context) {
	ret, err := func() (*models.Captcha, error) {
		if !config.Config.Captcha {
			return nil, errUnsupportCaptcha
		}

		driver := captchaDirver.ConvertFonts()
		id, content, answer := driver.GenerateIdQuestionAnswer()
		item, err := driver.DrawCaptcha(content)
		if err != nil {
			return nil, err
		}

		ret := &models.Captcha{
			CaptchaId: id,
			Answer:    answer,
			Image:     item.EncodeB64string(),
			CreatedAt: time.Now().Unix(),
		}

		if err := ret.Save(); err != nil {
			return nil, err
		}

		return ret, nil
	}()

	renderData(c, ret, err)
}

func authSettings(c *gin.Context) {
	renderData(c, struct {
		Sso bool `json:"sso"`
	}{
		Sso: config.Config.SSO.Enable,
	}, nil)
}
