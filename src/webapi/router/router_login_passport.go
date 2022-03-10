package router

import (
	"fmt"
	"net/http"
	"strings"
	"io/ioutil"

	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/webapi/config"
)

type loginPassportForm struct {
	PassportTicket string `json:"passport_ticket" binding:"required"`
}

func loginPassportPost(c *gin.Context) {
	var f loginPassportForm
	ginx.BindJSON(c, &f)

	if len(f.PassportTicket) == 0{
		ginx.NewRender(c).Message("PassPort KEY不能为空！")
		return
	}

	// passport_ticket认证
	username,err := getUsernameByPassportTicket(f.PassportTicket)
	if username == "0" {
		ginx.NewRender(c).Message("PassPort 无此用户!")
		return
	}
	if err != nil {
		ginx.NewRender(c).Message(err)
		return
	}
	user, err := models.PassPortLogin(username,config.C.PassportAuth.PassPortVerifyType)
	if err != nil {
		ginx.NewRender(c).Message(err)
		return
	}


	if user == nil {
		// Theoretically impossible
		ginx.NewRender(c).Message("PassPort认证失败，请联系管理员！")
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

func logoutPassportPost(c *gin.Context) {
	metadata, err := extractTokenMetadata(c.Request)
	if err != nil {
		ginx.NewRender(c, http.StatusBadRequest).Message("failed to parse jwt token")
		return
	}

	delErr := deleteTokens(c.Request.Context(), metadata)
	if delErr != nil {
		ginx.NewRender(c).Message(InternalServerError)
		return
	}

	ginx.NewRender(c).Message("")
}

type refreshPassportForm struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func refreshPassportPost(c *gin.Context) {
	var f refreshPassportForm
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
			ginx.NewRender(c, http.StatusUnauthorized).Message(InternalServerError)
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

func passportAuthCheckValid(c *gin.Context) {
	ticket := ginx.UrlParamStr(c, "passport_ticket")
	if len(ticket) == 0{
		ginx.NewRender(c).Message("PassPort KEY不能为空！")
		return
	}

	// passport_ticket认证
	username,err := getUsernameByPassportTicket(ticket)
	if username == "0" {
		ginx.NewRender(c).Message("PassPort 无此用户!")
		return
	}
	if err != nil {
		ginx.NewRender(c).Message(err)
		return
	}

	ginx.NewRender(c).Data(gin.H{
		"username":  username,
	}, nil)
}


func getUsernameByPassportTicket(passport_ticket string) (username string,err error) {
	// passport_ticket认证
	passport_url := config.C.PassportAuth.PassPortVerify
	if strings.Contains(passport_url, "?") {
		passport_url = passport_url + "&"
	}else {
		passport_url = passport_url + "?"
	}
	passport_url = passport_url + "passport_type=query&" + config.C.PassportAuth.PassportKeyName + "=" + passport_ticket

	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tr}

	resp, err := client.Get(passport_url)
	if resp != nil {
		defer resp.Body.Close()
	}

	if err != nil {
		return "",err
	}

	//fmt.Println(resp.StatusCode)

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "",err
	}

	username = string(body)
	if username == "0" {
		return username,nil
	}
	return username,nil
}