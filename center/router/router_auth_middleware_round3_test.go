package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/aop"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTokenAuth_ValidatesStoredSessionIdentity(t *testing.T) {
	cases := []struct {
		name           string
		identity       string
		expectedStatus int
	}{
		{name: "valid", identity: "1-alice", expectedStatus: http.StatusOK},
		{name: "missing separator", identity: "alice", expectedStatus: http.StatusUnauthorized},
		{name: "invalid user ID", identity: "bad-alice", expectedStatus: http.StatusUnauthorized},
		{name: "deleted user", identity: "404-alice", expectedStatus: http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, _ := sessionRouter(t)
			require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 1, Username: "alice"}).Error)
			tokens, err := rt.createTokens(rt.HTTP.JWTAuth.SigningKey, "1-alice")
			require.NoError(t, err)
			require.NoError(t, rt.Redis.Set(context.Background(), rt.wrapJwtKey(tokens.AccessUuid), tc.identity, 0).Err())
			engine := gin.New()
			engine.Use(aop.Recovery())
			engine.GET("/protected", rt.Auth(), func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"userid": c.GetInt64("userid"), "username": c.GetString("username")})
			})
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
			w := httptest.NewRecorder()

			engine.ServeHTTP(w, req)

			require.Equal(t, tc.expectedStatus, w.Code)
			if tc.expectedStatus == http.StatusOK {
				require.JSONEq(t, `{"userid":1,"username":"alice"}`, w.Body.String())
			} else {
				require.Contains(t, w.Body.String(), "unauthorized")
			}
		})
	}
}

func TestJwtMock_ProxyLoginReturnsUserAndAbortsHandler(t *testing.T) {
	rt := identityRouter(t, &models.User{})
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 1, Username: "alice"}).Error)
	rt.HTTP.ProxyAuth.Enable = true
	rt.HTTP.ProxyAuth.HeaderUserNameKey = "X-Remote-User"
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.POST("/login", rt.jwtMock(), func(c *gin.Context) { t.Error("proxy login continued into password handler") })
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.Header.Set("X-Remote-User", "alice")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	var got struct {
		User         models.User `json:"user"`
		AccessToken  string      `json:"access_token"`
		RefreshToken string      `json:"refresh_token"`
	}
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, "alice", got.User.Username)
	require.Empty(t, got.AccessToken)
	require.Empty(t, got.RefreshToken)
}

func TestJwtMock_RejectsLogoutWhenProxyAuthenticationIsEnabled(t *testing.T) {
	rt := &Router{}
	rt.HTTP.ProxyAuth.Enable = true
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.POST("/logout", rt.jwtMock(), func(c *gin.Context) { t.Error("proxy logout reached handler") })
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/logout", nil))

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "logout is not supported")
}

func TestJwtMock_DisabledProxyAuthenticationContinuesRequest(t *testing.T) {
	rt := &Router{}
	engine := gin.New()
	engine.GET("/login", rt.jwtMock(), func(c *gin.Context) { c.String(http.StatusOK, "password login") })
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/login", nil))

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "password login", w.Body.String())
}
