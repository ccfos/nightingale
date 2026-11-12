package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/center/cstats"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func mountedRouterEngine(t *testing.T, rt *Router) *gin.Engine {
	t.Helper()
	previous := cstats.RequestDuration
	cstats.RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Name: "router_round4_request_duration"}, []string{"code", "path", "method"},
	)
	t.Cleanup(func() { cstats.RequestDuration = previous })
	engine := gin.New()
	rt.Config(engine)
	return engine
}

func TestRouterConfig_ServiceAPIRequiresConfiguredBasicAuth(t *testing.T) {
	rt := identityRouter(t, &models.BusiGroup{})
	rt.HTTP.A2A.Disable = true
	rt.HTTP.APIForService.Enable = true
	rt.HTTP.APIForService.BasicAuth = gin.Accounts{"service": "local-password"}
	require.NoError(t, rt.Ctx.DB.Create(&models.BusiGroup{Id: 1, Name: "production"}).Error)
	engine := mountedRouterEngine(t, rt)
	cases := []struct {
		name     string
		password string
		status   int
	}{
		{name: "missing credentials", status: http.StatusUnauthorized},
		{name: "wrong credentials", password: "wrong", status: http.StatusUnauthorized},
		{name: "valid credentials", password: "local-password", status: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/n9e/busi-groups", nil)
			if tc.password != "" {
				req.SetBasicAuth("service", tc.password)
			}
			w := httptest.NewRecorder()

			engine.ServeHTTP(w, req)

			require.Equal(t, tc.status, w.Code, w.Body.String())
			if tc.status == http.StatusOK {
				var groups []models.BusiGroup
				require.Empty(t, identityReply(t, w, &groups))
				require.Len(t, groups, 1)
				require.Equal(t, "production", groups[0].Name)
			} else {
				require.Equal(t, `Basic realm="Authorization Required"`, w.Header().Get("WWW-Authenticate"))
				require.NotContains(t, w.Body.String(), "production")
			}
		})
	}
}

func TestRouterConfig_DisabledServiceAndAgentAPIsAreNotMounted(t *testing.T) {
	rt := &Router{}
	rt.HTTP.A2A.Disable = true
	engine := mountedRouterEngine(t, rt)

	routes := engine.Routes()

	for _, route := range routes {
		require.False(t, strings.HasPrefix(route.Path, "/v1/n9e/"), route.Path)
		require.False(t, strings.HasPrefix(route.Path, "/a2a"), route.Path)
		require.False(t, strings.HasPrefix(route.Path, "/mcp"), route.Path)
	}
}

func TestRouterConfig_AgentAPIRejectsUnauthenticatedHeartbeat(t *testing.T) {
	rt := &Router{}
	rt.HTTP.A2A.Disable = true
	rt.HTTP.APIForAgent.Enable = true
	rt.HTTP.APIForAgent.BasicAuth = gin.Accounts{"agent": "secret"}
	engine := mountedRouterEngine(t, rt)
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/n9e/heartbeat", strings.NewReader(`{"hostname":"host"}`)))

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.NotEmpty(t, w.Header().Get("WWW-Authenticate"))
	for _, route := range engine.Routes() {
		require.NotEqual(t, "/v1/n9e/users", route.Path)
	}
}

func TestRouterConfig_NotificationWritesEnforceOperationPermission(t *testing.T) {
	cases := []struct {
		name      string
		username  string
		operation string
		status    int
	}{
		{name: "anonymous", status: http.StatusUnauthorized},
		{name: "unbound role", username: "alice", status: http.StatusForbidden},
		{name: "wrong operation", username: "alice", operation: "/notification-rules", status: http.StatusForbidden},
		{name: "bound operation", username: "alice", operation: "/notification-rules/add", status: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.User{}, &models.RoleOperation{}, &models.NotifyRule{}, &models.UserGroupMember{})
			rt.HTTP.A2A.Disable = true
			rt.HTTP.ProxyAuth.Enable = true
			rt.HTTP.ProxyAuth.HeaderUserNameKey = "X-Remote-User"
			rt.UserCache = &memsto.UserCacheType{}
			require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 1, Username: "alice", Roles: "Operator"}).Error)
			require.NoError(t, rt.Ctx.DB.Create(&models.UserGroupMember{GroupId: 10, UserId: 1}).Error)
			if tc.operation != "" {
				require.NoError(t, rt.Ctx.DB.Create(&models.RoleOperation{RoleName: "Operator", Operation: tc.operation}).Error)
			}
			engine := mountedRouterEngine(t, rt)
			req := httptest.NewRequest(http.MethodPost, "/api/n9e/notify-rules",
				strings.NewReader(`[{"name":"critical","user_group_ids":[10],"enable":true}]`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Remote-User", tc.username)
			w := httptest.NewRecorder()

			engine.ServeHTTP(w, req)

			require.Equal(t, tc.status, w.Code, w.Body.String())
			var rules []models.NotifyRule
			require.NoError(t, rt.Ctx.DB.Find(&rules).Error)
			if tc.status == http.StatusOK {
				require.Empty(t, identityReply(t, w, nil))
				require.Len(t, rules, 1)
				require.Equal(t, "critical", rules[0].Name)
				require.Equal(t, "alice", rules[0].CreateBy)
				require.Equal(t, []int64{10}, rules[0].UserGroupIds)
			} else {
				require.Empty(t, rules)
			}
		})
	}
}

func TestRouterConfig_AnonymousDatasourceBriefHonorsAccessSwitch(t *testing.T) {
	for _, anonymous := range []bool{false, true} {
		name := "requires login"
		if anonymous {
			name = "public brief"
		}
		t.Run(name, func(t *testing.T) {
			rt := setupDatasourceSecretsTest(t)
			rt.HTTP.A2A.Disable = true
			rt.Center.AnonymousAccess.PromQuerier = anonymous
			engine := mountedRouterEngine(t, rt)
			w := httptest.NewRecorder()

			engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/n9e/datasource/brief", nil))

			if !anonymous {
				require.Equal(t, http.StatusUnauthorized, w.Code)
				return
			}
			var got []*models.Datasource
			require.Empty(t, identityReply(t, w, &got))
			require.Len(t, got, 2)
			for _, secret := range []string{"auth-password", "header-secret", "url-password", "settings-secret"} {
				require.NotContains(t, w.Body.String(), secret)
			}
		})
	}
}

func TestRouterConfig_AgentDiscoveryIsPublicButProtocolRequiresAuthentication(t *testing.T) {
	rt := &Router{}
	rt.HTTP.A2A.BaseURL = "https://n9e.example.invalid"
	engine := mountedRouterEngine(t, rt)
	for _, path := range []string{"/.well-known/agent-card.json", "/.well-known/agent.json"} {
		w := httptest.NewRecorder()

		engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))

		require.Equal(t, http.StatusOK, w.Code)
		var card map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &card))
		require.NotEmpty(t, card["name"])
		require.Contains(t, w.Body.String(), "https://n9e.example.invalid/a2a")
	}
	for _, path := range []string{"/a2a", "/a2a/message:send", "/mcp"} {
		w := httptest.NewRecorder()

		engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`)))

		require.Equal(t, http.StatusUnauthorized, w.Code, path)
		require.Contains(t, w.Body.String(), "unauthorized")
	}
}

func TestLanguageDetector_NormalizesConfiguredHeader(t *testing.T) {
	cases := []struct {
		name   string
		header string
		value  string
		want   string
	}{
		{name: "traditional Chinese", header: "Accept-Language", value: "zh_HK,en;q=0.9", want: "zh_HK"},
		{name: "Chinese", header: "Accept-Language", value: "zh_TW", want: "zh_CN"},
		{name: "English", header: "Accept-Language", value: "en-US", want: "en"},
		{name: "other language", header: "Accept-Language", value: "fr-FR", want: "fr-FR"},
		{name: "default", header: "Accept-Language", want: "zh_CN"},
		{name: "unconfigured", value: "en-US", want: "original"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := gin.New()
			engine.Use(languageDetector(tc.header))
			engine.GET("/language", func(c *gin.Context) { c.String(http.StatusOK, c.GetHeader("X-Language")) })
			req := httptest.NewRequest(http.MethodGet, "/language", nil)
			req.Header.Set("X-Language", "original")
			req.Header.Set("Accept-Language", tc.value)
			w := httptest.NewRecorder()

			engine.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, tc.want, w.Body.String())
		})
	}
}
