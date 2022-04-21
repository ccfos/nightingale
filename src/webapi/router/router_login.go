package router

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/pkg/oidcc"
	"github.com/didi/nightingale/v5/src/webapi/config"
)

type loginForm struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func loginPost(c *gin.Context) {
	var f loginForm
	ginx.BindJSON(c, &f)

	user, err := models.PassLogin(f.Username, f.Password)
	if err != nil {
		// pass validate fail, try ldap
		if config.C.LDAP.Enable {
			user, err = models.LdapLogin(f.Username, f.Password)
			if err != nil {
				ginx.NewRender(c).Message(err)
				return
			}
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

	ts, err := createTokens(config.C.JWTAuth.SigningKey, userIdentity)
	ginx.Dangerous(err)
	ginx.Dangerous(createAuth(c.Request.Context(), userIdentity, ts))

	ginx.NewRender(c).Data(gin.H{
		"user":          user,
		"access_token":  ts.AccessToken,
		"refresh_token": ts.RefreshToken,
	}, nil)
}

func logoutPost(c *gin.Context) {
	metadata, err := extractTokenMetadata(c.Request)
	if err != nil {
		ginx.NewRender(c, http.StatusBadRequest).Message("failed to parse jwt token")
		return
	}

	delErr := deleteTokens(c.Request.Context(), metadata)
	if delErr != nil {
		ginx.NewRender(c).Message(http.StatusText(http.StatusInternalServerError))
		return
	}

	ginx.NewRender(c).Message("")
}

type refreshForm struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func refreshPost(c *gin.Context) {
	var f refreshForm
	ginx.BindJSON(c, &f)

	// verify the token
	token, err := jwt.Parse(f.RefreshToken, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected jwt signing method: %v", token.Header["alg"])
		}
		return []byte(config.C.JWTAuth.SigningKey), nil
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

		// Delete the previous Refresh Token
		err = deleteAuth(c.Request.Context(), refreshUuid)
		if err != nil {
			ginx.NewRender(c, http.StatusUnauthorized).Message(http.StatusText(http.StatusInternalServerError))
			return
		}

		// Delete previous Access Token
		deleteAuth(c.Request.Context(), strings.Split(refreshUuid, "++")[0])

		// Create new pairs of refresh and access tokens
		ts, err := createTokens(config.C.JWTAuth.SigningKey, userIdentity)
		ginx.Dangerous(err)
		ginx.Dangerous(createAuth(c.Request.Context(), userIdentity, ts))

		ginx.NewRender(c).Data(gin.H{
			"access_token":  ts.AccessToken,
			"refresh_token": ts.RefreshToken,
		}, nil)
	} else {
		// redirect to login page
		ginx.NewRender(c, http.StatusUnauthorized).Message("refresh token expired")
	}
}

func loginRedirect(c *gin.Context) {
	redirect := ginx.QueryStr(c, "redirect", "/")

	v, exsits := c.Get("userid")
	if exsits {
		userid := v.(int64)
		user, err := models.UserGetById(userid)
		ginx.Dangerous(err)
		if user == nil {
			ginx.Bomb(200, "user not found")
		}

		if user.Username != "" { // alread login
			ginx.NewRender(c).Data(redirect, nil)
			return
		}
	}

	if !config.C.OIDC.Enable {
		ginx.NewRender(c).Data("", nil)
		return
	}

	redirect, err := oidcc.Authorize(redirect)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(redirect, err)
}

type CallbackOutput struct {
	Redirect     string       `json:"redirect"`
	User         *models.User `json:"user"`
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
}

func loginCallback(c *gin.Context) {
	code := ginx.QueryStr(c, "code", "")
	state := ginx.QueryStr(c, "state", "")

	ret, err := oidcc.Callback(c.Request.Context(), code, state)
	if err != nil {
		logger.Debugf("sso.callback() get ret %+v error %v", ret, err)
		ginx.NewRender(c).Data(CallbackOutput{}, err)
		return
	}

	user, err := models.UserGet("username=?", ret.Username)
	ginx.Dangerous(err)

	if user != nil {
		if config.C.OIDC.CoverAttributes {
			user.Nickname = ret.Nickname
			user.Email = ret.Email
			user.Phone = ret.Phone
			user.UpdateAt = time.Now().Unix()

			user.Update("email", "nickname", "phone", "update_at")
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
			Roles:    strings.Join(config.C.OIDC.DefaultRoles, " "),
			RolesLst: config.C.OIDC.DefaultRoles,
			Contacts: []byte("{}"),
			CreateAt: now,
			UpdateAt: now,
			CreateBy: "oidc",
			UpdateBy: "oidc",
		}

		// create user from oidc
		ginx.Dangerous(user.Add())
	}

	// set user login state
	userIdentity := fmt.Sprintf("%d-%s", user.Id, user.Username)
	ts, err := createTokens(config.C.JWTAuth.SigningKey, userIdentity)
	ginx.Dangerous(err)
	ginx.Dangerous(createAuth(c.Request.Context(), userIdentity, ts))

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
