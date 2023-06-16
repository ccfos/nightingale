package router

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/cas"
	"github.com/ccfos/nightingale/v6/pkg/ldapx"
	"github.com/ccfos/nightingale/v6/pkg/oauth2x"
	"github.com/ccfos/nightingale/v6/pkg/oidcx"
	"github.com/pelletier/go-toml/v2"

	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
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

	user, err := models.PassLogin(rt.Ctx, f.Username, f.Password)
	if err != nil {
		// pass validate fail, try ldap
		if rt.Sso.LDAP.Enable {
			roles := strings.Join(rt.Sso.LDAP.DefaultRoles, " ")
			user, err = models.LdapLogin(rt.Ctx, f.Username, f.Password, roles, rt.Sso.LDAP)
			if err != nil {
				logger.Debugf("ldap login failed: %v username: %s", err, f.Username)
				ginx.NewRender(c).Message(err)
				return
			}
			user.RolesLst = strings.Fields(user.Roles)
		} else {
			ginx.NewRender(c).Message(err)
			return
		}
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
	logger.Infof("username:%s login from:%s", c.GetString("username"), c.ClientIP())
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

	ginx.NewRender(c).Message("")
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
		logger.Debugf("sso.callback() get ret %+v error %v", ret, err)
		ginx.NewRender(c).Data(CallbackOutput{}, err)
		return
	}

	user, err := models.UserGet(rt.Ctx, "username=?", ret.Username)
	ginx.Dangerous(err)

	if user != nil {
		if rt.Sso.OIDC.CoverAttributes {
			if ret.Nickname != "" {
				user.Nickname = ret.Nickname
			}

			if ret.Email != "" {
				user.Email = ret.Email
			}

			if ret.Phone != "" {
				user.Phone = ret.Phone
			}

			user.UpdateAt = time.Now().Unix()
			user.Update(rt.Ctx, "email", "nickname", "phone", "update_at")
		}
	} else {
		now := time.Now().Unix()
		user = &models.User{
			Username: ret.Username,
			Password: "******",
			Nickname: ret.Nickname,
			Phone:    ret.Phone,
			Email:    ret.Email,
			Portrait: "",
			Roles:    strings.Join(rt.Sso.OIDC.DefaultRoles, " "),
			RolesLst: rt.Sso.OIDC.DefaultRoles,
			Contacts: []byte("{}"),
			CreateAt: now,
			UpdateAt: now,
			CreateBy: "oidc",
			UpdateBy: "oidc",
		}

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
			if ret.Nickname != "" {
				user.Nickname = ret.Nickname
			}

			if ret.Email != "" {
				user.Email = ret.Email
			}

			if ret.Phone != "" {
				user.Phone = ret.Phone
			}

			user.UpdateAt = time.Now().Unix()
			ginx.Dangerous(user.Update(rt.Ctx, "email", "nickname", "phone", "update_at"))
		}
	} else {
		now := time.Now().Unix()
		user = &models.User{
			Username: ret.Username,
			Password: "******",
			Nickname: ret.Nickname,
			Portrait: "",
			Roles:    strings.Join(rt.Sso.CAS.DefaultRoles, " "),
			RolesLst: rt.Sso.CAS.DefaultRoles,
			Contacts: []byte("{}"),
			Phone:    ret.Phone,
			Email:    ret.Email,
			CreateAt: now,
			UpdateAt: now,
			CreateBy: "CAS",
			UpdateBy: "CAS",
		}
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
			if ret.Nickname != "" {
				user.Nickname = ret.Nickname
			}

			if ret.Email != "" {
				user.Email = ret.Email
			}

			if ret.Phone != "" {
				user.Phone = ret.Phone
			}

			user.UpdateAt = time.Now().Unix()
			user.Update(rt.Ctx, "email", "nickname", "phone", "update_at")
		}
	} else {
		now := time.Now().Unix()
		user = &models.User{
			Username: ret.Username,
			Password: "******",
			Nickname: ret.Nickname,
			Phone:    ret.Phone,
			Email:    ret.Email,
			Portrait: "",
			Roles:    strings.Join(rt.Sso.OAuth2.DefaultRoles, " "),
			RolesLst: rt.Sso.OAuth2.DefaultRoles,
			Contacts: []byte("{}"),
			CreateAt: now,
			UpdateAt: now,
			CreateBy: "oauth2",
			UpdateBy: "oauth2",
		}

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
	ginx.NewRender(c).Data(SsoConfigOutput{
		OidcDisplayName:  rt.Sso.OIDC.GetDisplayName(),
		CasDisplayName:   rt.Sso.CAS.GetDisplayName(),
		OauthDisplayName: rt.Sso.OAuth2.GetDisplayName(),
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

		err = rt.Sso.OIDC.Reload(config)
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
