package router

import (
	"context"
	"github.com/gin-gonic/gin"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/dgrijalva/jwt-go"
	"github.com/stretchr/testify/require"
)

func TestR8cRefreshRejectsMalformedIdentityClaims(t *testing.T) {
	for _, tc := range []struct {
		name   string
		claims jwt.MapClaims
		msg    string
	}{
		{"missing refresh uuid", jwt.MapClaims{}, "refresh_uuid"},
		{"missing identity", jwt.MapClaims{"refresh_uuid": "refresh"}, "user_identity"},
		{"invalid identity type", jwt.MapClaims{"refresh_uuid": "refresh", "user_identity": 123}, "user_identity"},
		{"invalid userid", jwt.MapClaims{"refresh_uuid": "refresh", "user_identity": "bad-user"}, "user_identity"},
		{"deleted", jwt.MapClaims{"refresh_uuid": "refresh", "user_identity": "404-deleted"}, "user already deleted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt, mr := sessionRouter(t)
			mr.Set(rt.HTTP.JWTAuth.RedisKeyPrefix+"refresh", "404-deleted")
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, tc.claims).SignedString([]byte(rt.HTTP.JWTAuth.SigningKey))
			require.NoError(t, err)
			w := r8cRequest(t, rt.refreshPost, refreshForm{RefreshToken: token}, "/1")
			require.Equal(t, 401, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), tc.msg)
		})
	}
}
func TestR8cRefreshDatabaseFailure(t *testing.T) {
	rt, mr := sessionRouter(t)
	mr.Set(rt.HTTP.JWTAuth.RedisKeyPrefix+"refresh", "1-user")
	require.NoError(t, rt.Ctx.DB.Migrator().DropTable(&models.User{}))
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"refresh_uuid": "refresh", "user_identity": "1-user"}).SignedString([]byte(rt.HTTP.JWTAuth.SigningKey))
	require.NoError(t, err)
	w := r8cRequest(t, rt.refreshPost, refreshForm{RefreshToken: token}, "/1")
	require.Equal(t, 500, w.Code)
	require.Contains(t, w.Body.String(), "failed to query user")
}
func TestR8cLogoutRedisFailures(t *testing.T) {
	rt, mr := sessionRouter(t)
	ts, err := rt.createTokens(rt.HTTP.JWTAuth.SigningKey, "1-user")
	require.NoError(t, err)
	require.NoError(t, rt.createAuth(context.Background(), "1-user", ts))
	mr.SetError("ERR unavailable")
	w := identityRequest(t, rt, func(c *gin.Context) {
		c.Request.Header.Set("Authorization", "Bearer "+ts.AccessToken)
		rt.logoutPost(c)
	}, nil, "1", &models.User{Id: 1})
	require.Equal(t, 400, w.Code)
	require.Contains(t, w.Body.String(), "failed to parse jwt token")
}
