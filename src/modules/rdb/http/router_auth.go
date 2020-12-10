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
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/rdb/cache"
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

// login for UI
func login(c *gin.Context) {
	var in loginInput
	bind(c, &in)
	in.validate()

	in.RemoteAddr = c.ClientIP()

	if config.Config.Auth.Captcha {
		c, err := models.CaptchaGet("captcha_id=?", in.CaptchaId)
		dangerous(err)
		if strings.ToLower(c.Answer) != strings.ToLower(in.Answer) {
			dangerous(errInvalidAnswer)
		}
	}

	user, err := authLogin(in)
	dangerous(err)

	sessionSet(c, "username", user.Username)

	renderMessage(c, "")

	go models.LoginLogNew(user.Username, in.RemoteAddr, "in")
}

func logout(c *gin.Context) {
	func() {
		username := sessionUsername(c)
		if username == "" {
			return
		}
		sessionSet(c, "username", "")
		go models.LoginLogNew(username, c.ClientIP(), "out")
	}()

	if config.Config.SSO.Enable {
		redirect := queryStr(c, "redirect", "/")
		c.Redirect(302, ssoc.LogoutLocation(redirect))
		return
	}

	if redirect := queryStr(c, "redirect", ""); redirect != "" {
		c.Redirect(302, redirect)
		return
	}

	c.String(200, "logout successfully")
}

type authRedirect struct {
	Redirect string `json:"redirect"`
	Msg      string `json:"msg"`
}

func authAuthorizeV2(c *gin.Context) {
	err := sessionStart(c)
	dangerous(err)
	defer sessionUpdate(c)

	redirect := queryStr(c, "redirect", "/")
	ret := &authRedirect{Redirect: redirect}

	username := sessionUsername(c)
	if username != "" { // alread login
		renderData(c, ret, nil)
		return
	}

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
		logger.Debugf("sso.callback()  can't get code and redirect is not set")
		renderData(c, ret, nil)
		return
	}

	var user *models.User
	var err error
	ret.Redirect, user, err = ssoc.Callback(code, state)
	if err != nil {
		logger.Debugf("sso.callback() error %s", err)
		renderData(c, ret, err)
		return
	}

	dangerous(sessionStart(c))
	defer sessionUpdate(c)

	logger.Debugf("sso.callback() successfully, set username %s", user.Username)
	sessionSet(c, "username", user.Username)
	renderData(c, ret, nil)
}

func logoutV2(c *gin.Context) {
	sessionStart(c)

	redirect := queryStr(c, "redirect", "")
	ret := &authRedirect{Redirect: redirect}

	username := sessionUsername(c)
	if username == "" {
		renderData(c, ret, nil)
		return
	}

	sessionDestory(c)
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
			bomb("%s too long > 64", f.Username)
		}
	}
}

type v1LoginInput struct {
	Username   string   `json:"username"`
	Type       string   `param:"data" json:"type"`
	Args       []string `param:"data" json:"args"`
	RemoteAddr string   `param:"data" json:"remote_addr"`
}

// v1Login called by sso.rdb module
func v1Login(c *gin.Context) {
	var in v1LoginInput
	bind(c, &in)

	input := loginInput{
		Username:   in.Username,
		Type:       in.Type,
		RemoteAddr: in.RemoteAddr,
	}
	switch in.Type {
	case models.LOGIN_T_SMS:
		input.Phone = in.Args[0]
		input.Code = in.Args[1]
	case models.LOGIN_T_EMAIL:
		input.Email = in.Args[0]
		input.Code = in.Args[1]
	case models.LOGIN_T_PWD:
		input.Password = in.Args[0]
	}

	user, err := authLogin(input)
	renderData(c, *user, err)

	go models.LoginLogNew(user.Username, in.RemoteAddr, "in")
}

// authLogin called by /v1/rdb/login, /api/rdb/auth/login
func authLogin(in loginInput) (user *models.User, err error) {
	if config.Config.Auth.WhiteList {
		if err := models.WhiteListAccess(in.RemoteAddr); err != nil {
			return nil, err
		}
	}

	switch strings.ToLower(in.Type) {
	case models.LOGIN_T_LDAP:
		user, err = models.LdapLogin(in.Username, in.Password)
	case models.LOGIN_T_PWD:
		user, err = models.PassLogin(in.Username, in.Password)
	case models.LOGIN_T_SMS:
		user, err = models.SmsCodeLogin(in.Username, in.Phone, in.Code)
	case models.LOGIN_T_EMAIL:
		user, err = models.EmailCodeLogin(in.Username, in.Email, in.Code)
	default:
		return nil, fmt.Errorf("invalid login type %s", in.Type)
	}

	err = authPostCheck(in.Username, err == nil, user)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func authPostCheck(username string, login bool, user *models.User) (err error) {
	cookieName := config.Config.HTTP.Session.CookieName
	cf := cache.AuthConfig()
	if user == nil {
		if user, err = models.UserMustGet("username=?", username); err != nil {
			return err
		}
	}
	now := time.Now().Unix()
	defer func() {
		if err == nil {
			user.LoggedAt = now
		}
		user.Update("login_err_num", "status", "locked_at", "updated_at", "logged_at")
	}()

	if user.Typ == models.USER_T_TEMP && (now < user.ActiveBegin || user.ActiveEnd < now) {
		err = fmt.Errorf("Temporary user has expired")
		return
	}

	var n int64
retry:
	switch user.Status {
	case models.USER_S_ACTIVE:
		if cf.MaxNumErr > 0 && user.LoginErrNum >= cf.MaxNumErr {
			user.Status = models.USER_S_LOCKED
			user.LockedAt = now
			user.UpdatedAt = now
			goto retry
		}

		if !login {
			user.LoginErrNum++
			user.UpdatedAt = now
			err = fmt.Errorf("max login err %d/%d", user.LoginErrNum, cf.MaxNumErr)
			return err
		}

		user.LoginErrNum = 0
		user.UpdatedAt = now

		if cf.MaxSessionNumber > 0 {
			if n, err = models.SessionUserAll(cookieName, username); err != nil {
				return err
			}

			if n >= cf.MaxSessionNumber {
				err = fmt.Errorf("max session limit %d/%d", n, cf.MaxSessionNumber)
				return err
			}
		}

		if cf.PwdExpiresIn > 0 {
			if now-user.PwdUpdatedAt > cf.PwdExpiresIn {
				err = fmt.Errorf("password has been expired")
				return err
			}
		}
		return nil
	case models.USER_S_INACTIVE:
		err = fmt.Errorf("user is inactive")
	case models.USER_S_LOCKED:
		if now-user.LockedAt > cf.LockTime*60 {
			user.Status = models.USER_S_ACTIVE
			user.LoginErrNum = 0
			user.UpdatedAt = now
			goto retry
		}
		err = fmt.Errorf("user is locked")
	case models.USER_S_FROZEN:
		err = fmt.Errorf("user is frozen")
	case models.USER_S_WRITEN_OFF:
		err = fmt.Errorf("user is writen off")
	default:
		err = fmt.Errorf("invalid user status %d", user.Status)
	}
	return
}

type sendCodeInput struct {
	Username string `json:"username"`
	Type     string `json:"type" description:"sms-code, email-code"`
	Arg      string `json:"arg"`
}

func (p *sendCodeInput) Validate() error {
	if p.Username == "" {
		return fmt.Errorf("unable to get username")
	}
	if p.Type == "" {
		return fmt.Errorf("unable to get type, sms-code | email-code")
	}
	if p.Arg == "" {
		return fmt.Errorf("unable to get arg")
	}
	return nil
}

func sendLoginCode(c *gin.Context) {
	var in sendCodeInput
	bind(c, &in)

	msg, err := func() (string, error) {
		if !config.Config.Redis.Enable {
			return "", fmt.Errorf("sms/email sender is disabled")
		}

		if err := in.Validate(); err != nil {
			return "", err
		}

		// general a random code and add cache
		code := fmt.Sprintf("%06d", rand.Intn(1000000))

		loginCode := &models.LoginCode{
			Username:  in.Username,
			Code:      code,
			LoginType: in.Type,
			CreatedAt: time.Now().Unix(),
		}

		var (
			user      *models.User
			buf       bytes.Buffer
			queueName string
		)

		switch in.Type {
		case models.LOGIN_T_SMS:
			user, _ = models.UserGet("username=? and phone=?", in.Username, in.Arg)
			if err := loginCodeSmsTpl.Execute(&buf, loginCode); err != nil {
				return "", err
			}
			queueName = config.SMS_QUEUE_NAME
		case models.LOGIN_T_EMAIL:
			user, _ = models.UserGet("username=? and email=?", in.Username, in.Arg)
			if err := loginCodeEmailTpl.Execute(&buf, loginCode); err != nil {
				return "", err
			}
			queueName = config.MAIL_QUEUE_NAME
		default:
			return "", fmt.Errorf("invalid type %s", in.Type)
		}

		if user == nil {
			return "", fmt.Errorf("user informations is invalid")
		}

		if err := loginCode.Save(); err != nil {
			return "", err
		}

		if err := redisc.Write(&dataobj.Message{Tos: []string{in.Arg}, Content: buf.String()}, queueName); err != nil {
			return "", err
		}

		if debug {
			return fmt.Sprintf("[debug]: %s", buf.String()), nil
		}

		return "successed", nil

	}()
	renderData(c, msg, err)
}

func sendRstCode(c *gin.Context) {
	var in sendCodeInput
	bind(c, &in)

	msg, err := func() (string, error) {
		if !config.Config.Redis.Enable {
			return "", fmt.Errorf("email/sms sender is disabled")
		}

		if err := in.Validate(); err != nil {
			return "", err
		}

		// general a random code and add cache
		code := fmt.Sprintf("%06d", rand.Intn(1000000))

		loginCode := &models.LoginCode{
			Username:  in.Username,
			Code:      code,
			LoginType: models.LOGIN_T_RST,
			CreatedAt: time.Now().Unix(),
		}

		var (
			user      *models.User
			buf       bytes.Buffer
			queueName string
		)

		switch in.Type {
		case models.LOGIN_T_SMS:
			user, _ = models.UserGet("username=? and phone=?", in.Username, in.Arg)
			if err := loginCodeSmsTpl.Execute(&buf, loginCode); err != nil {
				return "", err
			}
			queueName = config.SMS_QUEUE_NAME
		case models.LOGIN_T_EMAIL:
			user, _ = models.UserGet("username=? and email=?", in.Username, in.Arg)
			if err := loginCodeEmailTpl.Execute(&buf, loginCode); err != nil {
				return "", err
			}
			queueName = config.MAIL_QUEUE_NAME
		default:
			return "", fmt.Errorf("invalid type %s", in.Type)
		}

		if user == nil {
			return "", fmt.Errorf("User %s's infomation is incorrect", in.Username)
		}

		if err := loginCode.Save(); err != nil {
			return "", err
		}

		if err := redisc.Write(&dataobj.Message{Tos: []string{in.Arg}, Content: buf.String()}, queueName); err != nil {
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
	Type     string `json:"type"`
	Arg      string `json:"arg"`
	Code     string `json:"code"`
	Password string `json:"password"`
	DryRun   bool   `json:"dryRun"`
}

func (p *rstPasswordInput) Validate() error {
	if p.Username == "" {
		return fmt.Errorf("unable to get username")
	}
	if p.Type == "" {
		return fmt.Errorf("unable to get type, sms-code | email-code")
	}
	if p.Arg == "" {
		return fmt.Errorf("unable to get arg")
	}
	if p.Password == "" {
		return fmt.Errorf("password must be set")
	}
	return nil
}

func rstPassword(c *gin.Context) {
	var in rstPasswordInput
	bind(c, &in)

	err := func() error {
		if err := in.Validate(); err != nil {
			return err
		}

		var user *models.User

		switch in.Type {
		case models.LOGIN_T_SMS:
			user, _ = models.UserGet("username=? and phone=?", in.Username, in.Arg)
		case models.LOGIN_T_EMAIL:
			user, _ = models.UserGet("username=? and email=?", in.Username, in.Arg)
		default:
			return fmt.Errorf("invalid type %s", in.Type)
		}

		if user == nil {
			return fmt.Errorf("User %s's infomation is incorrect", in.Username)
		}

		lc, err := models.LoginCodeGet("username=? and code=? and login_type=?",
			user.Username, in.Code, models.LOGIN_T_RST)
		if err != nil {
			return fmt.Errorf("invalid code")
		}

		if time.Now().Unix()-lc.CreatedAt > models.LOGIN_EXPIRES_IN {
			return fmt.Errorf("the code has expired")
		}

		if in.DryRun {
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
		if !config.Config.Auth.Captcha {
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

func authConfigsGet(c *gin.Context) {
	config, err := models.AuthConfigGet()
	renderData(c, config, err)
}

func authConfigsPut(c *gin.Context) {
	var in models.AuthConfig
	bind(c, &in)

	err := models.AuthConfigSet(&in)
	renderData(c, "", err)
}

type createWhiteListInput struct {
	StartIp   string `json:"startIp"`
	EndIp     string `json:"endIp"`
	StartTime int64  `json:"startTime"`
	EndTime   int64  `json:"endTime"`
}

func whiteListPost(c *gin.Context) {
	var in createWhiteListInput
	bind(c, &in)

	username := loginUser(c).Username
	ts := time.Now().Unix()

	wl := models.WhiteList{
		StartIp:   in.StartIp,
		EndIp:     in.EndIp,
		StartTime: in.StartTime,
		EndTime:   in.EndTime,
		CreatedAt: ts,
		UpdatedAt: ts,
		Creator:   username,
		Updater:   username,
	}
	if err := wl.Validate(); err != nil {
		bomb("invalid arguments %s", err)
	}
	dangerous(wl.Save())

	renderData(c, gin.H{"id": wl.Id}, nil)
}

func whiteListsGet(c *gin.Context) {
	limit := queryInt(c, "limit", 20)
	query := queryStr(c, "query", "")

	total, err := models.WhiteListTotal(query)
	dangerous(err)

	list, err := models.WhiteListGets(query, limit, offset(c, limit))
	dangerous(err)

	renderData(c, gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

func whiteListGet(c *gin.Context) {
	id := urlParamInt64(c, "id")
	ret, err := models.WhiteListGet("id=?", id)
	renderData(c, ret, err)
}

type updateWhiteListInput struct {
	StartIp   string `json:"startIp"`
	EndIp     string `json:"endIp"`
	StartTime int64  `json:"startTime"`
	EndTime   int64  `json:"endTime"`
}

func whiteListPut(c *gin.Context) {
	var in updateWhiteListInput
	bind(c, &in)

	wl, err := models.WhiteListGet("id=?", urlParamInt64(c, "id"))
	if err != nil {
		bomb("not found white list")
	}

	wl.StartIp = in.StartIp
	wl.EndIp = in.EndIp
	wl.StartTime = in.StartTime
	wl.EndTime = in.EndTime
	wl.UpdatedAt = time.Now().Unix()
	wl.Updater = loginUser(c).Username

	if err := wl.Validate(); err != nil {
		bomb("invalid arguments %s", err)
	}

	renderMessage(c, wl.Update("start_ip", "end_ip", "start_time", "end_time", "updated_at", "updater"))
}

func whiteListDel(c *gin.Context) {
	wl, err := models.WhiteListGet("id=?", urlParamInt64(c, "id"))
	dangerous(err)

	renderMessage(c, wl.Del())
}
