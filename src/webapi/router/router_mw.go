package router

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/storage"
	"github.com/didi/nightingale/v5/src/webapi/config"
)

type AccessDetails struct {
	AccessUuid   string
	UserIdentity string
}

func jwtAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		metadata, err := extractTokenMetadata(c.Request)
		if err != nil {
			ginx.Bomb(http.StatusUnauthorized, "unauthorized")
		}

		userIdentity, err := fetchAuth(c.Request.Context(), metadata.AccessUuid)
		if err != nil {
			ginx.Bomb(http.StatusUnauthorized, "unauthorized")
		}

		// ${userid}-${username}
		arr := strings.SplitN(userIdentity, "-", 2)
		if len(arr) != 2 {
			ginx.Bomb(http.StatusUnauthorized, "unauthorized")
		}

		userid, err := strconv.ParseInt(arr[0], 10, 64)
		if err != nil {
			ginx.Bomb(http.StatusUnauthorized, "unauthorized")
		}

		c.Set("userid", userid)
		c.Set("username", arr[1])

		c.Next()
	}
}

func user() gin.HandlerFunc {
	return func(c *gin.Context) {
		userid := c.MustGet("userid").(int64)

		user, err := models.UserGetById(userid)
		if err != nil {
			ginx.Bomb(http.StatusUnauthorized, "unauthorized")
		}

		if user == nil {
			ginx.Bomb(http.StatusUnauthorized, "unauthorized")
		}

		c.Set("user", user)
		c.Next()
	}
}

func userGroupWrite() gin.HandlerFunc {
	return func(c *gin.Context) {
		me := c.MustGet("user").(*models.User)
		ug := UserGroup(ginx.UrlParamInt64(c, "id"))

		can, err := me.CanModifyUserGroup(ug)
		ginx.Dangerous(err)

		if !can {
			ginx.Bomb(http.StatusForbidden, "forbidden")
		}

		c.Set("user_group", ug)
		c.Next()
	}
}

func bgro() gin.HandlerFunc {
	return func(c *gin.Context) {
		me := c.MustGet("user").(*models.User)
		bg := BusiGroup(ginx.UrlParamInt64(c, "id"))

		can, err := me.CanDoBusiGroup(bg)
		ginx.Dangerous(err)

		if !can {
			ginx.Bomb(http.StatusForbidden, "forbidden")
		}

		c.Set("busi_group", bg)
		c.Next()
	}
}

// bgrw 逐步要被干掉，不安全
func bgrw() gin.HandlerFunc {
	return func(c *gin.Context) {
		me := c.MustGet("user").(*models.User)
		bg := BusiGroup(ginx.UrlParamInt64(c, "id"))

		can, err := me.CanDoBusiGroup(bg, "rw")
		ginx.Dangerous(err)

		if !can {
			ginx.Bomb(http.StatusForbidden, "forbidden")
		}

		c.Set("busi_group", bg)
		c.Next()
	}
}

// bgrwCheck 要逐渐替换掉bgrw方法，更安全
func bgrwCheck(c *gin.Context, bgid int64) {
	me := c.MustGet("user").(*models.User)
	bg := BusiGroup(bgid)

	can, err := me.CanDoBusiGroup(bg, "rw")
	ginx.Dangerous(err)

	if !can {
		ginx.Bomb(http.StatusForbidden, "forbidden")
	}

	c.Set("busi_group", bg)
}

func perm(operation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		me := c.MustGet("user").(*models.User)

		can, err := me.CheckPerm(operation)
		ginx.Dangerous(err)

		if !can {
			ginx.Bomb(http.StatusForbidden, "forbidden")
		}

		c.Next()
	}
}

func admin() gin.HandlerFunc {
	return func(c *gin.Context) {
		userid := c.MustGet("userid").(int64)

		user, err := models.UserGetById(userid)
		if err != nil {
			ginx.Bomb(http.StatusUnauthorized, "unauthorized")
		}

		if user == nil {
			ginx.Bomb(http.StatusUnauthorized, "unauthorized")
		}

		roles := strings.Fields(user.Roles)
		found := false
		for i := 0; i < len(roles); i++ {
			if roles[i] == config.C.AdminRole {
				found = true
				break
			}
		}

		if !found {
			ginx.Bomb(http.StatusForbidden, "forbidden")
		}

		c.Set("user", user)
		c.Next()
	}
}

func extractTokenMetadata(r *http.Request) (*AccessDetails, error) {
	token, err := verifyToken(config.C.JWTAuth.SigningKey, extractToken(r))
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if ok && token.Valid {
		accessUuid, ok := claims["access_uuid"].(string)
		if !ok {
			return nil, err
		}

		return &AccessDetails{
			AccessUuid:   accessUuid,
			UserIdentity: claims["user_identity"].(string),
		}, nil
	}

	return nil, err
}

func extractToken(r *http.Request) string {
	tok := r.Header.Get("Authorization")

	if len(tok) > 6 && strings.ToUpper(tok[0:7]) == "BEARER " {
		return tok[7:]
	}

	return ""
}

func createAuth(ctx context.Context, userIdentity string, td *TokenDetails) error {
	at := time.Unix(td.AtExpires, 0)
	rt := time.Unix(td.RtExpires, 0)
	now := time.Now()

	errAccess := storage.Redis.Set(ctx, wrapJwtKey(td.AccessUuid), userIdentity, at.Sub(now)).Err()
	if errAccess != nil {
		return errAccess
	}

	errRefresh := storage.Redis.Set(ctx, wrapJwtKey(td.RefreshUuid), userIdentity, rt.Sub(now)).Err()
	if errRefresh != nil {
		return errRefresh
	}

	return nil
}

func fetchAuth(ctx context.Context, givenUuid string) (string, error) {
	return storage.Redis.Get(ctx, wrapJwtKey(givenUuid)).Result()
}

func deleteAuth(ctx context.Context, givenUuid string) error {
	return storage.Redis.Del(ctx, wrapJwtKey(givenUuid)).Err()
}

func deleteTokens(ctx context.Context, authD *AccessDetails) error {
	// get the refresh uuid
	refreshUuid := authD.AccessUuid + "++" + authD.UserIdentity

	// delete access token
	err := storage.Redis.Del(ctx, wrapJwtKey(authD.AccessUuid)).Err()
	if err != nil {
		return err
	}

	// delete refresh token
	err = storage.Redis.Del(ctx, wrapJwtKey(refreshUuid)).Err()
	if err != nil {
		return err
	}

	return nil
}

func wrapJwtKey(key string) string {
	return config.C.JWTAuth.RedisKeyPrefix + key
}

type TokenDetails struct {
	AccessToken  string
	RefreshToken string
	AccessUuid   string
	RefreshUuid  string
	AtExpires    int64
	RtExpires    int64
}

func createTokens(signingKey, userIdentity string) (*TokenDetails, error) {
	td := &TokenDetails{}
	td.AtExpires = time.Now().Add(time.Minute * time.Duration(config.C.JWTAuth.AccessExpired)).Unix()
	td.AccessUuid = uuid.NewString()

	td.RtExpires = time.Now().Add(time.Minute * time.Duration(config.C.JWTAuth.RefreshExpired)).Unix()
	td.RefreshUuid = td.AccessUuid + "++" + userIdentity

	var err error
	// Creating Access Token
	atClaims := jwt.MapClaims{}
	atClaims["authorized"] = true
	atClaims["access_uuid"] = td.AccessUuid
	atClaims["user_identity"] = userIdentity
	atClaims["exp"] = td.AtExpires
	at := jwt.NewWithClaims(jwt.SigningMethodHS256, atClaims)
	td.AccessToken, err = at.SignedString([]byte(signingKey))
	if err != nil {
		return nil, err
	}

	// Creating Refresh Token
	rtClaims := jwt.MapClaims{}
	rtClaims["refresh_uuid"] = td.RefreshUuid
	rtClaims["user_identity"] = userIdentity
	rtClaims["exp"] = td.RtExpires
	rt := jwt.NewWithClaims(jwt.SigningMethodHS256, rtClaims)
	td.RefreshToken, err = rt.SignedString([]byte(signingKey))
	if err != nil {
		return nil, err
	}

	return td, nil
}

func verifyToken(signingKey, tokenString string) (*jwt.Token, error) {
	if tokenString == "" {
		return nil, fmt.Errorf("Bearer token not found")
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected jwt signing method: %v", token.Header["alg"])
		}
		return []byte(signingKey), nil
	})
	if err != nil {
		return nil, err
	}
	return token, nil
}
