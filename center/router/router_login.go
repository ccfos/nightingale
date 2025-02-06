package router

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/cas"
	"github.com/ccfos/nightingale/v6/pkg/ldapx"
	"github.com/ccfos/nightingale/v6/pkg/oauth2x"
	"github.com/ccfos/nightingale/v6/pkg/oidcx"
	"github.com/ccfos/nightingale/v6/pkg/secu"

	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/pelletier/go-toml/v2"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
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
	switch user.Belong {
	case "oidc":
		logoutAddr = rt.Sso.OIDC.GetSsoLogoutAddr()
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
	OidcDisplayName  string `json:"oidcDisplayName"`
	CasDisplayName   string `json:"casDisplayName"`
	OauthDisplayName string `json:"oauthDisplayName"`
}

func (rt *Router) ssoConfigNameGet(c *gin.Context) {
	var oidcDisplayName, casDisplayName, oauthDisplayName string
	if rt.Sso.OIDC != nil {
		oidcDisplayName = rt.Sso.OIDC.GetDisplayName()
	}

	if rt.Sso.CAS != nil {
		casDisplayName = rt.Sso.CAS.GetDisplayName()
	}

	if rt.Sso.OAuth2 != nil {
		oauthDisplayName = rt.Sso.OAuth2.GetDisplayName()
	}

	ginx.NewRender(c).Data(SsoConfigOutput{
		OidcDisplayName:  oidcDisplayName,
		CasDisplayName:   casDisplayName,
		OauthDisplayName: oauthDisplayName,
	}, nil)
}

func (rt *Router) ssoConfigGets(c *gin.Context) {
	ginx.NewRender(c).Data(models.SsoConfigGets(rt.Ctx))
}

func (rt *Router) ssoConfigUpdate(c *gin.Context) {
	var f models.SsoConfig
	ginx.BindJSON(c, &f)

	err := f.Update(rt.Ctx)
	ginx.Dangerous(err)

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
