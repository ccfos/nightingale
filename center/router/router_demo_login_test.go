package router

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/httpx"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func setupDemoLoginTest(t *testing.T) *Router {
	t.Helper()
	gin.SetMode(gin.TestMode)

	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}))
	for _, u := range []models.User{
		{Username: "demo", Roles: "Guest", Contacts: []byte("{}")},
		{Username: "root", Roles: models.AdminRole, Contacts: []byte("{}")},
		{Username: "frozen", Roles: "Guest", Disabled: models.UserDisabled, Contacts: []byte("{}")},
	} {
		require.NoError(t, db.Create(&u).Error)
	}

	rt := &Router{
		Ctx:   &ctx.Context{DB: db},
		Redis: redis.NewClient(&redis.Options{Addr: mr.Addr()}),
	}
	rt.HTTP.JWTAuth = httpx.JWTAuth{SigningKey: "session-signing-key", AccessExpired: 10, RefreshExpired: 60}
	return rt
}

func doDemoLogin(rt *Router) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/n9e/auth/demo-login", nil)
	rt.demoLogin(c)
	return w
}

func TestDemoLogin(t *testing.T) {
	t.Run("disabled by default", func(t *testing.T) {
		rt := setupDemoLoginTest(t)
		rt.Center.DemoLogin.Username = "demo"
		w := doDemoLogin(rt)
		require.Equal(t, http.StatusNotFound, w.Code)
		require.NotContains(t, w.Body.String(), "access_token")
	})

	t.Run("signs in the demo user", func(t *testing.T) {
		rt := setupDemoLoginTest(t)
		rt.Center.DemoLogin = cconf.DemoLogin{Enable: true, Username: "demo", RedirectURL: "/dashboards"}
		w := doDemoLogin(rt)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		require.True(t, strings.HasPrefix(w.Header().Get("Content-Type"), "text/html"))
		require.Contains(t, w.Body.String(), `location.replace("/dashboards")`)

		m := regexp.MustCompile(`setItem\('access_token', "([^"]+)"\)`).FindStringSubmatch(w.Body.String())
		require.Len(t, m, 2)

		// 签出的 token 必须是一个真实可用的会话
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+m[1])
		metadata, err := rt.extractTokenMetadata(req)
		require.NoError(t, err)
		userIdentity, err := rt.fetchAuth(req.Context(), metadata.AccessUuid)
		require.NoError(t, err)
		require.Equal(t, "1-demo", userIdentity)
	})

	for _, username := range []string{"root", "frozen", "nobody", ""} {
		t.Run("rejects user "+username, func(t *testing.T) {
			rt := setupDemoLoginTest(t)
			rt.Center.DemoLogin = cconf.DemoLogin{Enable: true, Username: username}
			w := doDemoLogin(rt)
			require.Equal(t, http.StatusForbidden, w.Code)
			require.NotContains(t, w.Body.String(), "access_token")
		})
	}
}

func TestDemoLoginRedirectURL(t *testing.T) {
	for in, want := range map[string]string{
		"":                     "/",
		"/":                    "/",
		"/dashboards/1?a=b":    "/dashboards/1?a=b",
		"//evil.example":       "/",
		"/\\evil.example":      "/",
		"https://evil.example": "/",
		"javascript:alert(1)":  "/",
	} {
		require.Equal(t, want, demoLoginRedirectURL(in), in)
	}
}
