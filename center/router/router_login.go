package router

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/cas"
	"github.com/ccfos/nightingale/v6/pkg/dingtalk"
	"github.com/ccfos/nightingale/v6/pkg/ldapx"
	"github.com/ccfos/nightingale/v6/pkg/oauth2x"
	"github.com/ccfos/nightingale/v6/pkg/oidcx"
	"github.com/ccfos/nightingale/v6/pkg/secu"

	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/pelletier/go-toml/v2"
	"github.com/pkg/errors"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
	"gorm.io/gorm"
)

type loginForm struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	Captchaid   string `json:"captchaid"`
	Verifyvalue string `json:"verifyvalue"`
}

func (rt *Router) loginPost(c *gin.Context) {
	var f loginForm
	ginx.BindJSON(c, &f)
	logger.Infof("username:%s login from:%s", f.Username, c.ClientIP())

	if rt.HTTP.ShowCaptcha.Enable {
		if !CaptchaVerify(f.Captchaid, f.Verifyvalue) {
			ginx.NewRender(c).Message("incorrect verification code")
			return
		}
	}
	authPassWord := f.Password
	// need decode
	if rt.HTTP.RSA.OpenRSA {
		decPassWord, err := secu.Decrypt(f.Password, rt.HTTP.RSA.RSAPrivateKey, rt.HTTP.RSA.RSAPassWord)
		if err != nil {
			logger.Errorf("RSA Decrypt failed: %v username: %s", err, f.Username)
			ginx.NewRender(c).Message(err)
			return
		}
		authPassWord = decPassWord
	}

	var user *models.User
	var err error
	lc := rt.Sso.LDAP.Copy()
	if lc.Enable {
		user, err = ldapx.LdapLogin(rt.Ctx, f.Username, authPassWord, lc.DefaultRoles, lc.DefaultTeams, lc)
		if err != nil {
			logger.Debugf("ldap login failed: %v username: %s", err, f.Username)
			var errLoginInN9e error
			// to use n9e as the minimum guarantee for login
			if user, errLoginInN9e = models.PassLogin(rt.Ctx, rt.Redis, f.Username, authPassWord); errLoginInN9e != nil {
				ginx.NewRender(c).Message("ldap login failed: %v; n9e login failed: %v", err, errLoginInN9e)
				return
			}
		} else {
			user.RolesLst = strings.Fields(user.Roles)
		}
	} else {
		user, err = models.PassLogin(rt.Ctx, rt.Redis, f.Username, authPassWord)
		ginx.Dangerous(err)
	}

	if user == nil {
		// Theoretically impossible
		ginx.NewRender(c).Message("Username or password invalid")
		return
	}

	userIdentity := fmt.Sprintf("%d-%s", user.Id, user.Username)

	ts, err := rt.createTokens(rt.HTTP.JWTAuth.SigningKey, userIdentity)
	ginx.Dangerous(err)
	ginx.Dangerous(rt.createAuth(c.Request.Context(), userIdentity, ts))

	ginx.NewRender(c).Data(gin.H{
		"user":          user,
		"access_token":  ts.AccessToken,
		"refresh_token": ts.RefreshToken,
	}, nil)
}

func (rt *Router) logoutPost(c *gin.Context) {
	logger.Infof("username:%s logout from:%s", c.GetString("username"), c.ClientIP())
	metadata, err := rt.extractTokenMetadata(c.Request)
	if err != nil {
		ginx.NewRender(c, http.StatusBadRequest).Message("failed to parse jwt token")
		return
	}

	delErr := rt.deleteTokens(c.Request.Context(), metadata)
	if delErr != nil {
		ginx.NewRender(c).Message(http.StatusText(http.StatusInternalServerError))
		return
	}

	var logoutAddr string
	user := c.MustGet("user").(*models.User)

	// 获取用户的 id_token
	idToken, err := rt.fetchIdToken(c.Request.Context(), user.Id)
	if err != nil {
		logger.Debugf("fetch id_token failed: %v, user_id: %d", err, user.Id)
		idToken = "" // 如果获取失败，使用空字符串
	}

	// 删除 id_token
	rt.deleteIdToken(c.Request.Context(), user.Id)

	switch user.Belong {
	case "oidc":
		logoutAddr = rt.Sso.OIDC.GetSsoLogoutAddr(idToken)
	case "cas":
		logoutAddr = rt.Sso.CAS.GetSsoLogoutAddr()
	case "oauth2":
		logoutAddr = rt.Sso.OAuth2.GetSsoLogoutAddr()
	}

	ginx.NewRender(c).Data(logoutAddr, nil)
}

type refreshForm struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func (rt *Router) refreshPost(c *gin.Context) {
	var f refreshForm
	ginx.BindJSON(c, &f)

	// verify the token
	token, err := jwt.Parse(f.RefreshToken, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected jwt signing method: %v", token.Header["alg"])
		}
		return []byte(rt.HTTP.JWTAuth.SigningKey), nil
	})

	// if there is an error, the token must have expired
	if err != nil {
		// redirect to login page
		ginx.NewRender(c, http.StatusUnauthorized).Message("refresh token expired")
		return
	}

	// Since token is valid, get the uuid:
	claims, ok := token.Claims.(jwt.MapClaims) //the token claims should conform to MapClaims
	if ok && token.Valid {
		refreshUuid, ok := claims["refresh_uuid"].(string) //convert the interface to string
		if !ok {
			// Theoretically impossible
			ginx.NewRender(c, http.StatusUnauthorized).Message("failed to parse refresh_uuid from jwt")
			return
		}

		// 看这个 token 是否还存在 redis 中
		val, err := rt.fetchAuth(c.Request.Context(), refreshUuid)
		if err != nil || val == "" {
			ginx.NewRender(c, http.StatusUnauthorized).Message("refresh token expired")
			return
		}

		userIdentity, ok := claims["user_identity"].(string)
		if !ok {
			// Theoretically impossible
			ginx.NewRender(c, http.StatusUnauthorized).Message("failed to parse user_identity from jwt")
			return
		}

		userid, err := strconv.ParseInt(strings.Split(userIdentity, "-")[0], 10, 64)
		if err != nil {
			ginx.NewRender(c, http.StatusUnauthorized).Message("failed to parse user_identity from jwt")
			return
		}

		u, err := models.UserGetById(rt.Ctx, userid)
		if err != nil {
			ginx.NewRender(c, http.StatusInternalServerError).Message("failed to query user by id")
			return
		}

		if u == nil {
			// user already deleted
			ginx.NewRender(c, http.StatusUnauthorized).Message("user already deleted")
			return
		}

		// Delete the previous Refresh Token
		err = rt.deleteAuth(c.Request.Context(), refreshUuid)
		if err != nil {
			ginx.NewRender(c, http.StatusUnauthorized).Message(http.StatusText(http.StatusInternalServerError))
			return
		}

		// Delete previous Access Token
		rt.deleteAuth(c.Request.Context(), strings.Split(refreshUuid, "++")[0])

		// Create new pairs of refresh and access tokens
		ts, err := rt.createTokens(rt.HTTP.JWTAuth.SigningKey, userIdentity)
		ginx.Dangerous(err)
		ginx.Dangerous(rt.createAuth(c.Request.Context(), userIdentity, ts))

		// 延长 id_token 的过期时间，使其与新的 refresh token 生命周期保持一致
		// 注意：这里不会获取新的 id_token，只是延长 Redis 中现有 id_token 的 TTL
		if idToken, err := rt.fetchIdToken(c.Request.Context(), userid); err == nil && idToken != "" {
			if err := rt.saveIdToken(c.Request.Context(), userid, idToken); err != nil {
				logger.Debugf("refresh id_token ttl failed: %v, user_id: %d", err, userid)
			}
		}

		ginx.NewRender(c).Data(gin.H{
			"access_token":  ts.AccessToken,
			"refresh_token": ts.RefreshToken,
		}, nil)
	} else {
		// redirect to login page
		ginx.NewRender(c, http.StatusUnauthorized).Message("refresh token expired")
	}
}

func (rt *Router) loginRedirect(c *gin.Context) {
	redirect := ginx.QueryStr(c, "redirect", "/")

	v, exists := c.Get("userid")
	if exists {
		userid := v.(int64)
		user, err := models.UserGetById(rt.Ctx, userid)
		ginx.Dangerous(err)
		if user == nil {
			ginx.Bomb(200, "user not found")
		}

		if user.Username != "" { // already login
			ginx.NewRender(c).Data(redirect, nil)
			return
		}
	}

	if !rt.Sso.OIDC.Enable {
		ginx.NewRender(c).Data("", nil)
		return
	}

	redirect, err := rt.Sso.OIDC.Authorize(rt.Redis, redirect)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(redirect, err)
}

type CallbackOutput struct {
	Redirect     string       `json:"redirect"`
	User         *models.User `json:"user"`
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
}

func (rt *Router) loginCallback(c *gin.Context) {
	code := ginx.QueryStr(c, "code", "")
	state := ginx.QueryStr(c, "state", "")

	ret, err := rt.Sso.OIDC.Callback(rt.Redis, c.Request.Context(), code, state)
	if err != nil {
		logger.Errorf("sso_callback fail. code:%s, state:%s, get ret: %+v. error: %v", code, state, ret, err)
		ginx.NewRender(c).Data(CallbackOutput{}, err)
		return
	}

	user, err := models.UserGet(rt.Ctx, "username=?", ret.Username)
	ginx.Dangerous(err)

	if user != nil {
		if rt.Sso.OIDC.CoverAttributes {
			updatedFields := user.UpdateSsoFields("oidc", ret.Nickname, ret.Phone, ret.Email)
			ginx.Dangerous(user.Update(rt.Ctx, "update_at", updatedFields...))
		}
	} else {
		user = new(models.User)
		user.FullSsoFields("oidc", ret.Username, ret.Nickname, ret.Phone, ret.Email, rt.Sso.OIDC.DefaultRoles)
		// create user from oidc
		ginx.Dangerous(user.Add(rt.Ctx))

		if len(rt.Sso.OIDC.DefaultTeams) > 0 {
			for _, gid := range rt.Sso.OIDC.DefaultTeams {
				err = models.UserGroupMemberAdd(rt.Ctx, gid, user.Id)
				if err != nil {
					logger.Errorf("user:%v UserGroupMemberAdd: %s", user, err)
				}
			}
		}
	}

	// set user login state
	userIdentity := fmt.Sprintf("%d-%s", user.Id, user.Username)
	ts, err := rt.createTokens(rt.HTTP.JWTAuth.SigningKey, userIdentity)
	ginx.Dangerous(err)
	ginx.Dangerous(rt.createAuth(c.Request.Context(), userIdentity, ts))

	// 保存 id_token 到 Redis，用于登出时使用
	if ret.IdToken != "" {
		if err := rt.saveIdToken(c.Request.Context(), user.Id, ret.IdToken); err != nil {
			logger.Errorf("save id_token failed: %v, user_id: %d", err, user.Id)
		}
	}

	redirect := "/"
	if ret.Redirect != "/login" {
		redirect = ret.Redirect
	}

	ginx.NewRender(c).Data(CallbackOutput{
		Redirect:     redirect,
		User:         user,
		AccessToken:  ts.AccessToken,
		RefreshToken: ts.RefreshToken,
	}, nil)
}

type RedirectOutput struct {
	Redirect string `json:"redirect"`
	State    string `json:"state"`
}

func (rt *Router) loginRedirectCas(c *gin.Context) {
	redirect := ginx.QueryStr(c, "redirect", "/")

	v, exists := c.Get("userid")
	if exists {
		userid := v.(int64)
		user, err := models.UserGetById(rt.Ctx, userid)
		ginx.Dangerous(err)
		if user == nil {
			ginx.Bomb(200, "user not found")
		}

		if user.Username != "" { // already login
			ginx.NewRender(c).Data(redirect, nil)
			return
		}
	}

	if !rt.Sso.CAS.Enable {
		logger.Error("cas is not enable")
		ginx.NewRender(c).Data("", nil)
		return
	}

	redirect, state, err := rt.Sso.CAS.Authorize(rt.Redis, redirect)

	ginx.Dangerous(err)
	ginx.NewRender(c).Data(RedirectOutput{
		Redirect: redirect,
		State:    state,
	}, err)
}

func (rt *Router) loginCallbackCas(c *gin.Context) {
	ticket := ginx.QueryStr(c, "ticket", "")
	state := ginx.QueryStr(c, "state", "")
	ret, err := rt.Sso.CAS.ValidateServiceTicket(c.Request.Context(), ticket, state, rt.Redis)
	if err != nil {
		logger.Errorf("ValidateServiceTicket: %s", err)
		ginx.NewRender(c).Data("", err)
		return
	}
	user, err := models.UserGet(rt.Ctx, "username=?", ret.Username)
	if err != nil {
		logger.Errorf("UserGet: %s", err)
	}
	ginx.Dangerous(err)
	if user != nil {
		if rt.Sso.CAS.CoverAttributes {
			updatedFields := user.UpdateSsoFields("cas", ret.Nickname, ret.Phone, ret.Email)
			ginx.Dangerous(user.Update(rt.Ctx, "update_at", updatedFields...))
		}
	} else {
		user = new(models.User)
		user.FullSsoFields("cas", ret.Username, ret.Nickname, ret.Phone, ret.Email, rt.Sso.CAS.DefaultRoles)
		// create user from cas
		ginx.Dangerous(user.Add(rt.Ctx))
	}

	// set user login state
	userIdentity := fmt.Sprintf("%d-%s", user.Id, user.Username)
	ts, err := rt.createTokens(rt.HTTP.JWTAuth.SigningKey, userIdentity)
	if err != nil {
		logger.Errorf("createTokens: %s", err)
	}
	ginx.Dangerous(err)
	ginx.Dangerous(rt.createAuth(c.Request.Context(), userIdentity, ts))

	redirect := "/"
	if ret.Redirect != "/login" {
		redirect = ret.Redirect
	}
	ginx.NewRender(c).Data(CallbackOutput{
		Redirect:     redirect,
		User:         user,
		AccessToken:  ts.AccessToken,
		RefreshToken: ts.RefreshToken,
	}, nil)
}

func (rt *Router) loginRedirectOAuth(c *gin.Context) {
	redirect := ginx.QueryStr(c, "redirect", "/")

	v, exists := c.Get("userid")
	if exists {
		userid := v.(int64)
		user, err := models.UserGetById(rt.Ctx, userid)
		ginx.Dangerous(err)
		if user == nil {
			ginx.Bomb(200, "user not found")
		}

		if user.Username != "" { // already login
			ginx.NewRender(c).Data(redirect, nil)
			return
		}
	}

	if !rt.Sso.OAuth2.Enable {
		ginx.NewRender(c).Data("", nil)
		return
	}

	redirect, err := rt.Sso.OAuth2.Authorize(rt.Redis, redirect)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(redirect, err)
}

func (rt *Router) loginRedirectDingTalk(c *gin.Context) {
	redirect := ginx.QueryStr(c, "redirect", "/")

	v, exists := c.Get("userid")
	if exists {
		userid := v.(int64)
		user, err := models.UserGetById(rt.Ctx, userid)
		ginx.Dangerous(err)
		if user == nil {
			ginx.Bomb(200, "user not found")
		}

		if user.Username != "" { // already login
			ginx.NewRender(c).Data(redirect, nil)
			return
		}
	}

	if !rt.Sso.DingTalk.Enable {
		ginx.NewRender(c).Data("", nil)
		return
	}

	redirect, err := rt.Sso.DingTalk.Authorize(rt.Redis, redirect)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(redirect, err)
}

func (rt *Router) loginCallbackDingTalk(c *gin.Context) {
	code := ginx.QueryStr(c, "code", "")
	state := ginx.QueryStr(c, "state", "")

	ret, err := rt.Sso.DingTalk.Callback(rt.Redis, c.Request.Context(), code, state)
	if err != nil {
		logger.Errorf("sso_callback DingTalk fail. code:%s, state:%s, get ret: %+v. error: %v", code, state, ret, err)
		ginx.NewRender(c).Data(CallbackOutput{}, err)
		return
	}

	user, err := models.UserGet(rt.Ctx, "username=?", ret.Username)
	ginx.Dangerous(err)

	if user != nil {
		if rt.Sso.DingTalk.DingTalkConfig.CoverAttributes {
			updatedFields := user.UpdateSsoFields(dingtalk.SsoTypeName, ret.Nickname, ret.Phone, ret.Email)
			ginx.Dangerous(user.Update(rt.Ctx, "update_at", updatedFields...))
		}
	} else {
		user = new(models.User)
		user.FullSsoFields(dingtalk.SsoTypeName, ret.Username, ret.Nickname, ret.Phone, ret.Email, rt.Sso.DingTalk.DingTalkConfig.DefaultRoles)
		// create user from dingtalk
		ginx.Dangerous(user.Add(rt.Ctx))
	}

	// set user login state
	userIdentity := fmt.Sprintf("%d-%s", user.Id, user.Username)
	ts, err := rt.createTokens(rt.HTTP.JWTAuth.SigningKey, userIdentity)
	ginx.Dangerous(err)
	ginx.Dangerous(rt.createAuth(c.Request.Context(), userIdentity, ts))

	redirect := "/"
	if ret.Redirect != "/login" {
		redirect = ret.Redirect
	}

	ginx.NewRender(c).Data(CallbackOutput{
		Redirect:     redirect,
		User:         user,
		AccessToken:  ts.AccessToken,
		RefreshToken: ts.RefreshToken,
	}, nil)

}

func (rt *Router) loginCallbackOAuth(c *gin.Context) {
	code := ginx.QueryStr(c, "code", "")
	state := ginx.QueryStr(c, "state", "")

	ret, err := rt.Sso.OAuth2.Callback(rt.Redis, c.Request.Context(), code, state)
	if err != nil {
		logger.Debugf("sso.callback() get ret %+v error %v", ret, err)
		ginx.NewRender(c).Data(CallbackOutput{}, err)
		return
	}

	user, err := models.UserGet(rt.Ctx, "username=?", ret.Username)
	ginx.Dangerous(err)

	if user != nil {
		if rt.Sso.OAuth2.CoverAttributes {
			updatedFields := user.UpdateSsoFields("oauth2", ret.Nickname, ret.Phone, ret.Email)
			ginx.Dangerous(user.Update(rt.Ctx, "update_at", updatedFields...))
		}
	} else {
		user = new(models.User)
		user.FullSsoFields("oauth2", ret.Username, ret.Nickname, ret.Phone, ret.Email, rt.Sso.OAuth2.DefaultRoles)
		// create user from oidc
		ginx.Dangerous(user.Add(rt.Ctx))
	}

	// set user login state
	userIdentity := fmt.Sprintf("%d-%s", user.Id, user.Username)
	ts, err := rt.createTokens(rt.HTTP.JWTAuth.SigningKey, userIdentity)
	ginx.Dangerous(err)
	ginx.Dangerous(rt.createAuth(c.Request.Context(), userIdentity, ts))

	redirect := "/"
	if ret.Redirect != "/login" {
		redirect = ret.Redirect
	}

	ginx.NewRender(c).Data(CallbackOutput{
		Redirect:     redirect,
		User:         user,
		AccessToken:  ts.AccessToken,
		RefreshToken: ts.RefreshToken,
	}, nil)
}

type SsoConfigOutput struct {
	OidcDisplayName     string `json:"oidcDisplayName"`
	CasDisplayName      string `json:"casDisplayName"`
	OauthDisplayName    string `json:"oauthDisplayName"`
	DingTalkDisplayName string `json:"dingTalkDisplayName"`
}

func (rt *Router) ssoConfigNameGet(c *gin.Context) {
	var oidcDisplayName, casDisplayName, oauthDisplayName, dingTalkDisplayName string
	if rt.Sso.OIDC != nil {
		oidcDisplayName = rt.Sso.OIDC.GetDisplayName()
	}

	if rt.Sso.CAS != nil {
		casDisplayName = rt.Sso.CAS.GetDisplayName()
	}

	if rt.Sso.OAuth2 != nil {
		oauthDisplayName = rt.Sso.OAuth2.GetDisplayName()
	}

	if rt.Sso.DingTalk != nil {
		dingTalkDisplayName = rt.Sso.DingTalk.GetDisplayName()
	}

	ginx.NewRender(c).Data(SsoConfigOutput{
		OidcDisplayName:     oidcDisplayName,
		CasDisplayName:      casDisplayName,
		OauthDisplayName:    oauthDisplayName,
		DingTalkDisplayName: dingTalkDisplayName,
	}, nil)
}

func (rt *Router) ssoConfigGets(c *gin.Context) {
	var ssoConfigs []models.SsoConfig
	lst, err := models.SsoConfigGets(rt.Ctx)
	ginx.Dangerous(err)
	if len(lst) == 0 {
		ginx.NewRender(c).Data(ssoConfigs, nil)
		return
	}

	// TODO: dingTalkExist 为了兼容当前前端配置, 后期单点登陆统一调整后不在预先设置默认内容
	dingTalkExist := false
	for _, config := range lst {
		var ssoReqConfig models.SsoConfig
		ssoReqConfig.Id = config.Id
		ssoReqConfig.Name = config.Name
		ssoReqConfig.UpdateAt = config.UpdateAt
		switch config.Name {
		case dingtalk.SsoTypeName:
			dingTalkExist = true
			err := json.Unmarshal([]byte(config.Content), &ssoReqConfig.SettingJson)
			ginx.Dangerous(err)
		default:
			ssoReqConfig.Content = config.Content
		}

		ssoConfigs = append(ssoConfigs, ssoReqConfig)
	}
	// TODO: dingTalkExist 为了兼容当前前端配置, 后期单点登陆统一调整后不在预先设置默认内容
	if !dingTalkExist {
		var ssoConfig models.SsoConfig
		ssoConfig.Name = dingtalk.SsoTypeName
		ssoConfigs = append(ssoConfigs, ssoConfig)
	}

	ginx.NewRender(c).Data(ssoConfigs, nil)
}

func (rt *Router) ssoConfigUpdate(c *gin.Context) {
	var f models.SsoConfig
	var ssoConfig models.SsoConfig
	ginx.BindJSON(c, &ssoConfig)

	switch ssoConfig.Name {
	case dingtalk.SsoTypeName:
		f.Name = ssoConfig.Name
		setting, err := json.Marshal(ssoConfig.SettingJson)
		ginx.Dangerous(err)
		f.Content = string(setting)
		f.UpdateAt = time.Now().Unix()
		sso, err := f.Query(rt.Ctx)
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			ginx.Dangerous(err)
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = f.Create(rt.Ctx)
		} else {
			f.Id = sso.Id
			err = f.Update(rt.Ctx)
		}
		ginx.Dangerous(err)
	default:
		f.Id = ssoConfig.Id
		f.Name = ssoConfig.Name
		f.Content = ssoConfig.Content
		err := f.Update(rt.Ctx)
		ginx.Dangerous(err)
	}

	switch f.Name {
	case "LDAP":
		var config ldapx.Config
		err := toml.Unmarshal([]byte(f.Content), &config)
		ginx.Dangerous(err)
		rt.Sso.LDAP.Reload(config)
	case "OIDC":
		var config oidcx.Config
		err := toml.Unmarshal([]byte(f.Content), &config)
		ginx.Dangerous(err)
		rt.Sso.OIDC, err = oidcx.New(config)
		ginx.Dangerous(err)
	case "CAS":
		var config cas.Config
		err := toml.Unmarshal([]byte(f.Content), &config)
		ginx.Dangerous(err)
		rt.Sso.CAS.Reload(config)
	case "OAuth2":
		var config oauth2x.Config
		err := toml.Unmarshal([]byte(f.Content), &config)
		ginx.Dangerous(err)
		rt.Sso.OAuth2.Reload(config)
	case dingtalk.SsoTypeName:
		var config dingtalk.Config
		err := json.Unmarshal([]byte(f.Content), &config)
		ginx.Dangerous(err)
		if rt.Sso.DingTalk == nil {
			rt.Sso.DingTalk = dingtalk.New(config)
		}
		rt.Sso.DingTalk.Reload(config)
	}

	ginx.NewRender(c).Message(nil)
}

type RSAConfigOutput struct {
	OpenRSA      bool
	RSAPublicKey string
}

func (rt *Router) rsaConfigGet(c *gin.Context) {
	publicKey := ""
	if len(rt.HTTP.RSA.RSAPublicKey) > 0 {
		publicKey = base64.StdEncoding.EncodeToString(rt.HTTP.RSA.RSAPublicKey)
	}
	ginx.NewRender(c).Data(RSAConfigOutput{
		OpenRSA:      rt.HTTP.RSA.OpenRSA,
		RSAPublicKey: publicKey,
	}, nil)
}
