package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/rdb/cache"
	"github.com/didi/nightingale/src/modules/rdb/config"
	"github.com/didi/nightingale/src/modules/rdb/ssoc"
	"github.com/didi/nightingale/src/toolkits/i18n"
	pkgcache "github.com/toolkits/pkg/cache"
	"github.com/toolkits/pkg/logger"
)

const (
	ChangePasswordURL = "/change-password"
	loginModeFifo     = true
)

type Authenticator struct {
	extraMode     bool
	whiteList     bool
	debug         bool
	debugUser     string
	frozenTime    int64
	writenOffTime int64
	userExpire    bool

	ctx    context.Context
	cancel context.CancelFunc
}

// description:"enable user expire control, active -> frozen -> writen-off"
func New(cf config.AuthExtraSection) *Authenticator {
	if !cf.Enable {
		return &Authenticator{}
	}

	return &Authenticator{
		extraMode:     true,
		whiteList:     cf.WhiteList,
		debug:         cf.Debug,
		debugUser:     cf.DebugUser,
		frozenTime:    86400 * int64(cf.FrozenDays),
		writenOffTime: 86400 * int64(cf.WritenOffDays),
	}
}

func (p *Authenticator) WhiteListAccess(user *models.User, remoteAddr string) error {
	if !p.extraMode || !p.whiteList || (p.debug && user.Username != p.debugUser) {
		return nil
	}

	if err := models.WhiteListAccess(remoteAddr); err != nil {
		return err
	}
	return nil
}

func (p *Authenticator) PostLogin(user *models.User, loginErr error) (err error) {
	now := time.Now().Unix()
	defer func() {
		if user == nil {
			return
		}
		if err == nil {
			user.LoggedAt = now
		}
		user.Update("status", "login_err_num", "locked_at", "updated_at", "logged_at")
	}()

	if !p.extraMode || user == nil || (p.debug && user.Username != p.debugUser) {
		err = loginErr
		return
	}

	cf := cache.AuthConfig()

	if user.Type == models.USER_T_TEMP && (now < user.ActiveBegin || user.ActiveEnd < now) {
		err = _e("Temporary user has expired")
		return
	}

	status := user.Status
retry:
	switch user.Status {
	case models.USER_S_ACTIVE:
		err = activeUserAccess(cf, user, loginErr)
	case models.USER_S_INACTIVE:
		err = inactiveUserAccess(cf, user, loginErr)
	case models.USER_S_LOCKED:
		err = lockedUserAccess(cf, user, loginErr)
	case models.USER_S_FROZEN:
		err = frozenUserAccess(cf, user, loginErr)
	case models.USER_S_WRITEN_OFF:
		err = writenOffUserAccess(cf, user, loginErr)
	default:
		err = _e("Invalid user status %d", user.Status)
	}

	// if user's status has been changed goto retry
	if user.Status != status {
		status = user.Status
		goto retry
	}
	return
}

func (p *Authenticator) ChangePassword(user *models.User, password string) (err error) {
	defer func() {
		if err == nil {
			err = user.Update("password", "passwords",
				"pwd_updated_at", "updated_at")
		}
	}()

	changePassword := func() error {
		pwd, err := models.CryptoPass(password)
		if err != nil {
			return err
		}

		now := time.Now().Unix()
		user.Password = pwd
		user.PwdUpdatedAt = now
		user.UpdatedAt = now
		return nil
	}

	if !p.extraMode {
		return changePassword()
	}

	// precheck
	cf := cache.AuthConfig()
	if err = checkPassword(cf, password); err != nil {
		return
	}

	if err = changePassword(); err != nil {
		return
	}

	var passwords []string
	err = json.Unmarshal([]byte(user.Passwords), &passwords)
	if err != nil {
		// reset passwords
		passwords = []string{user.Password}
		b, _ := json.Marshal(passwords)
		user.Passwords = string(b)
		err = nil
		return
	}

	for _, v := range passwords {
		if user.Password == v {
			err = _e("The password is the same as the old password")
			return
		}
	}

	passwords = append(passwords, user.Password)
	if n := len(passwords) - cf.PwdHistorySize; n > 0 {
		passwords = passwords[n:]
	}

	b, _ := json.Marshal(passwords)
	user.Passwords = string(b)
	return
}

func (p *Authenticator) CheckPassword(password string) error {
	if !p.extraMode {
		return nil
	}
	return checkPassword(cache.AuthConfig(), password)
}

// PostCallback between sso.Callback() and sessionLogin()
func (p *Authenticator) PostCallback(in *ssoc.CallbackOutput) error {
	if !p.extraMode || (p.debug && in.User.Username != p.debugUser) {
		return nil
	}

	cf := cache.AuthConfig()

	if err := p.changePasswordRedirect(in, cf); err != nil {
		return err
	}

	// check user session limit
	tokens := []models.Token{}
	if maxCnt := int(cf.MaxSessionNumber); maxCnt > 0 {
		models.DB["sso"].SQL("select * from token where user_name=? order by id desc", in.User.Username).Find(&tokens)

		if n := len(tokens); n > maxCnt {
			for i := maxCnt; i < n; i++ {
				logger.Debugf("[over limit] delete session by token %s %s", tokens[i].UserName, tokens[i].AccessToken)
				deleteSessionByToken(&tokens[i])
			}
		}
	}

	return nil
}

// ChangePasswordRedirect check user should change password before login
// return err when need changePassword
func (p *Authenticator) changePasswordRedirect(in *ssoc.CallbackOutput, cf *models.AuthConfig) (err error) {
	if in.User.PwdUpdatedAt == 0 {
		err = _e("First Login, please change the password in time")
	} else if cf.PwdExpiresIn > 0 && in.User.PwdUpdatedAt+cf.PwdExpiresIn*86400*30 < time.Now().Unix() {
		err = _e("Password expired, please change the password in time")
	}

	if err != nil {
		v := url.Values{
			"redirect": {in.Redirect},
			"username": {in.User.Username},
			"reason":   {err.Error()},
			"pwdRules": cf.PwdRules(),
		}
		in.Redirect = ChangePasswordURL + "?" + v.Encode()
	}
	return
}

func (p *Authenticator) DeleteSession(sid string) error {
	s, err := models.SessionGet(sid)
	if err != nil {
		return err
	}

	if !p.extraMode {
		pkgcache.Delete("sid." + s.Sid)
		return models.SessionDelete(s.Sid)
	}
	return deleteSession(s)
}

func (p *Authenticator) DeleteToken(accessToken string) error {
	if !p.extraMode {
		return nil
	}
	token, err := models.TokenGet(accessToken)
	if err != nil {
		return err
	}
	return deleteSessionByToken(token)
}

func (p *Authenticator) Stop() error {
	p.cancel()
	return nil
}

func (p *Authenticator) Start() error {
	p.ctx, p.cancel = context.WithCancel(context.Background())

	if !p.extraMode {
		return nil
	}

	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-p.ctx.Done():
				return
			case <-t.C:
				p.cleanupSession()
			}
		}
	}()

	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()

		for {
			select {
			case <-p.ctx.Done():
				return
			case <-t.C:
				p.updateUserStatus()
			}
		}
	}()
	return nil
}

// cleanup rdb.session & sso.token
func (p *Authenticator) cleanupSession() {
	now := time.Now().Unix()
	cf := cache.AuthConfig()

	// idle session cleanup
	if cf.MaxConnIdleTime > 0 {
		expiresAt := now - cf.MaxConnIdleTime*60
		sessions := []models.Session{}
		if err := models.DB["rdb"].SQL("select * from session where updated_at < ? and username <> '' ", expiresAt).Find(&sessions); err != nil {
			logger.Errorf("token idle time cleanup err %s", err)
		}

		logger.Debugf("find %d idle sessions that should be clean up", len(sessions))

		for _, s := range sessions {
			if p.debug && s.Username != p.debugUser {
				continue
			}

			logger.Debugf("[idle] deleteSession %s %s", s.Username, s.Sid)
			deleteSession(&s)
		}
	}

	// session count limit cleanup
	if maxCnt := int(cf.MaxSessionNumber); maxCnt > 0 {
		tokens := []models.Token{}
		userName := ""
		cnt := 0

		if err := models.DB["sso"].SQL("select * from token order by user_name, id desc").Find(&tokens); err != nil {
			logger.Errorf("token idle time cleanup err %s", err)
		}

		for _, token := range tokens {
			if userName != token.UserName {
				userName = token.UserName
				cnt = 0
			}

			cnt++
			if cnt > maxCnt {
				if p.debug && token.UserName != p.debugUser {
					continue
				}
				logger.Debugf("[over limit] deleteSessionByToken %s %s idx %d max %d", token.UserName, token.AccessToken, cnt, maxCnt)
				deleteSessionByToken(&token)
			}
		}
	}
}

func (p *Authenticator) updateUserStatus() {
	now := time.Now().Unix()
	if p.frozenTime > 0 {
		// 3个月以上未登录，用户自动变为休眠状态
		if _, err := models.DB["rdb"].Exec("update user set status=?, updated_at=?, locked_at=? where ((logged_at > 0 and logged_at<?) or (logged_at == 0 and created_at < ?)) and status in (?,?,?)",
			models.USER_S_FROZEN, now, now, now-p.frozenTime,
			models.USER_S_ACTIVE, models.USER_S_INACTIVE, models.USER_S_LOCKED); err != nil {
			logger.Errorf("update user status error %s", err)
		}
	}

	if p.writenOffTime > 0 {
		// 变为休眠状态后1年未激活，用户自动变为已注销状态
		if _, err := models.DB["rdb"].Exec("update user set status=?, updated_at=? where locked_at<? and status=?",
			models.USER_S_WRITEN_OFF, now, now-p.writenOffTime, models.USER_S_FROZEN); err != nil {
			logger.Errorf("update user status error %s", err)
		}
	}

	// reset login err num before 24 hours ago
	if _, err := models.DB["rdb"].Exec("update user set login_err_num=0, updated_at=? where updated_at<? and login_err_num>0", now, now-86400); err != nil {
		logger.Errorf("update user login err num error %s", err)
	}

}

func activeUserAccess(cf *models.AuthConfig, user *models.User, loginErr error) error {
	now := time.Now().Unix()

	if loginErr != nil {
		if cf.MaxNumErr > 0 {
			user.UpdatedAt = now
			user.LoginErrNum++
			if user.LoginErrNum >= cf.MaxNumErr {
				user.Status = models.USER_S_LOCKED
				user.LockedAt = now
				return nil
			}
			return _e("Incorrect login/password %s times, you still have %s chances",
				user.LoginErrNum, cf.MaxNumErr-user.LoginErrNum)
		} else {
			return loginErr
		}
	}

	user.LoginErrNum = 0
	user.UpdatedAt = now

	if cf.MaxSessionNumber > 0 && !loginModeFifo {
		if n, err := models.SessionUserAll(user.Username); err != nil {
			return err
		} else if n >= cf.MaxSessionNumber {
			return _e("The limited sessions %d", cf.MaxSessionNumber)
		}
	}

	return nil
}
func inactiveUserAccess(cf *models.AuthConfig, user *models.User, loginErr error) error {
	return _e("User is inactive")
}
func lockedUserAccess(cf *models.AuthConfig, user *models.User, loginErr error) error {
	now := time.Now().Unix()
	if now-user.LockedAt > cf.LockTime*60 {
		user.Status = models.USER_S_ACTIVE
		user.LoginErrNum = 0
		user.UpdatedAt = now
		return nil
	}
	return _e("User is locked")
}

func frozenUserAccess(cf *models.AuthConfig, user *models.User, loginErr error) error {
	return _e("User is frozen")
}

func writenOffUserAccess(cf *models.AuthConfig, user *models.User, loginErr error) error {
	return _e("User is writen off")
}

func checkPassword(cf *models.AuthConfig, passwd string) error {
	indNum := [4]int{0, 0, 0, 0}
	spCode := []byte{'!', '@', '#', '$', '%', '^', '&', '*', '_', '-', '~', '.', ',', '<', '>', '/', ';', ':', '|', '?', '+', '='}

	if cf.PwdMinLenght > 0 && len(passwd) < cf.PwdMinLenght {
		return _e("Password too short (min:%d) %s", cf.PwdMinLenght, cf.MustInclude())
	}

	passwdByte := []byte(passwd)

	for _, i := range passwdByte {

		if i >= 'A' && i <= 'Z' {
			indNum[0] = 1
			continue
		}

		if i >= 'a' && i <= 'z' {
			indNum[1] = 1
			continue
		}

		if i >= '0' && i <= '9' {
			indNum[2] = 1
			continue
		}

		has := false
		for _, s := range spCode {
			if i == s {
				indNum[3] = 1
				has = true
				break
			}
		}

		if !has {
			return _e("character: %s not supported", string(i))
		}
	}

	if cf.PwdMustIncludeFlag&models.PWD_INCLUDE_UPPER > 0 && indNum[0] == 0 {
		return _e("Invalid Password, %s", cf.MustInclude())
	}

	if cf.PwdMustIncludeFlag&models.PWD_INCLUDE_LOWER > 0 && indNum[1] == 0 {
		return _e("Invalid Password, %s", cf.MustInclude())
	}

	if cf.PwdMustIncludeFlag&models.PWD_INCLUDE_NUMBER > 0 && indNum[2] == 0 {
		return _e("Invalid Password, %s", cf.MustInclude())
	}

	if cf.PwdMustIncludeFlag&models.PWD_INCLUDE_SPEC_CHAR > 0 && indNum[3] == 0 {
		return _e("Invalid Password, %s", cf.MustInclude())
	}

	return nil
}

func deleteSession(s *models.Session) error {
	pkgcache.Delete("sid." + s.Sid)
	pkgcache.Delete("access-token." + s.AccessToken)

	if err := models.SessionDelete(s.Sid); err != nil {
		return err
	}
	return models.TokenDelete(s.AccessToken)
}

func deleteSessionByToken(t *models.Token) error {
	if s, _ := models.SessionGetByToken(t.AccessToken); s != nil {
		deleteSession(s)
	} else {
		pkgcache.Delete("access-token." + t.AccessToken)
		models.TokenDelete(t.AccessToken)
	}

	return nil
}

func _e(format string, a ...interface{}) error {
	return fmt.Errorf(i18n.Sprintf(format, a...))
}

func _s(format string, a ...interface{}) string {
	return i18n.Sprintf(format, a...)
}
