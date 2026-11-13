package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/aop"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
	"github.com/stretchr/testify/require"
)

func TestGetXUserToken_CreatesTokenForExistingUser(t *testing.T) {
	rt := identityRouter(t, &models.User{}, &models.UserToken{})
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Username: "alice"}).Error)

	w := identityRequest(t, rt, rt.getXUserToken, xUserTokenReq{Username: "alice"}, "0", nil)

	var got models.UserToken
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, "alice", got.Username)
	require.Equal(t, "user-token", got.TokenName)
	require.NotEmpty(t, got.Token)
	var stored []models.UserToken
	require.NoError(t, rt.Ctx.DB.Find(&stored).Error)
	require.Equal(t, []models.UserToken{got}, stored)
}

func TestGetXUserToken_ReusesExistingUserToken(t *testing.T) {
	rt := identityRouter(t, &models.User{}, &models.UserToken{})
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Username: "alice"}).Error)
	original := models.UserToken{Username: "alice", TokenName: "automation", Token: "existing-token", CreateAt: 123}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)

	w := identityRequest(t, rt, rt.getXUserToken, xUserTokenReq{Username: "alice"}, "0", nil)

	var got models.UserToken
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, original, got)
	var stored []models.UserToken
	require.NoError(t, rt.Ctx.DB.Find(&stored).Error)
	require.Equal(t, []models.UserToken{original}, stored)
}

func TestGetXUserToken_RejectsInvalidIdentityWithoutCreatingTokens(t *testing.T) {
	cases := []struct {
		name    string
		form    xUserTokenReq
		oauth   bool
		status  int
		message string
	}{
		{name: "empty", status: http.StatusBadRequest, message: "raw_token or username required"},
		{
			name: "unknown user", form: xUserTokenReq{Username: "missing"},
			status: http.StatusNotFound, message: "user not found",
		},
		{
			name: "oauth disabled", form: xUserTokenReq{RawToken: "invalid", Username: "alice"},
			status: http.StatusServiceUnavailable, message: "oauth is not enabled",
		},
		{
			name: "invalid oauth cannot fall back to username", form: xUserTokenReq{RawToken: "invalid", Username: "alice"},
			oauth: true, status: http.StatusUnauthorized, message: "unauthorized",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.User{}, &models.UserToken{})
			rt.HTTP.MCPAuth = httpx.MCPAuth{Enable: tc.oauth, SigningKey: "test-signing-key"}
			require.NoError(t, rt.Ctx.DB.Create(&models.User{Username: "alice"}).Error)

			w := identityRequest(t, rt, rt.getXUserToken, tc.form, "0", nil)

			require.Equal(t, tc.status, w.Code)
			require.Contains(t, w.Body.String(), tc.message)
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.UserToken{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestGetXUserToken_UsesVerifiedOAuthIdentityInsteadOfClaimedUsername(t *testing.T) {
	rt := identityRouter(t, &models.User{}, &models.UserToken{})
	rt.HTTP.MCPAuth = httpx.MCPAuth{Enable: true, SigningKey: "test-signing-key"}
	require.NoError(t, rt.Ctx.DB.Create(&[]models.User{{Id: 1, Username: "alice"}, {Id: 2, Username: "bob"}}).Error)
	foreign := models.UserToken{Username: "bob", TokenName: "private", Token: "bob-secret"}
	require.NoError(t, rt.Ctx.DB.Create(&foreign).Error)
	raw, err := rt.mcpSign(jwt.MapClaims{"token_use": mcpUseAccess, "sub": "1", "usr": "alice"})
	require.NoError(t, err)

	w := identityRequest(t, rt, rt.getXUserToken, xUserTokenReq{RawToken: raw, Username: "bob"}, "0", nil)

	var got models.UserToken
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, "alice", got.Username)
	require.NotEmpty(t, got.Token)
	require.NotContains(t, w.Body.String(), "bob-secret")
	var stored models.UserToken
	require.NoError(t, rt.Ctx.DB.First(&stored, foreign.Id).Error)
	require.Equal(t, foreign, stored)
}

func TestServiceUser_PopulatesAuthenticatedRequestContext(t *testing.T) {
	rt := identityRouter(t, &models.User{})
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 7, Username: "alice", Nickname: "Alice"}).Error)
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.GET("/service", rt.serviceUser(), func(c *gin.Context) {
		user := c.MustGet("user").(*models.User)
		c.JSON(http.StatusOK, gin.H{
			"userid": c.GetInt64("userid"), "username": c.GetString("username"), "nickname": user.Nickname,
		})
	})
	req := httptest.NewRequest(http.MethodGet, "/service", nil)
	req.Header.Set("X-Service-Username", "alice")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"userid":7,"username":"alice","nickname":"Alice"}`, w.Body.String())
}

func TestServiceUser_RejectsMissingAndUnknownUsersBeforeHandler(t *testing.T) {
	cases := []struct {
		name     string
		username string
		status   int
		message  string
	}{
		{name: "missing header", status: http.StatusBadRequest, message: "X-Service-Username header is required"},
		{name: "unknown user", username: "missing", status: http.StatusNotFound, message: "user not found: missing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.User{})
			engine := gin.New()
			engine.Use(aop.Recovery())
			engine.GET("/service", rt.serviceUser(), func(c *gin.Context) { t.Error("rejected request reached handler") })
			req := httptest.NewRequest(http.MethodGet, "/service", nil)
			req.Header.Set("X-Service-Username", tc.username)
			w := httptest.NewRecorder()

			engine.ServeHTTP(w, req)

			require.Equal(t, tc.status, w.Code)
			require.Contains(t, w.Body.String(), tc.message)
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.User{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}
