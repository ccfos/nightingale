package cas

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func casRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return server, client
}

func TestNew_CopiesEnabledConfiguration(t *testing.T) {
	cf := Config{
		Enable: true, SsoAddr: "https://sso.example/cas", SsoLogoutAddr: "/logout",
		RedirectURL: "https://n9e.example/callback",
		DisplayName: "Corporate CAS", CoverAttributes: true, DefaultRoles: []string{"viewer"}, SkipTlsVerify: true,
	}
	cf.Attributes.UserName, cf.Attributes.Nickname = "uid", "displayName"
	cf.Attributes.Phone, cf.Attributes.Email = "mobile", "mail"

	client := New(cf)

	require.True(t, client.Enable)
	require.Equal(t, cf, client.Config)
	require.Equal(t, cf.SsoAddr, client.SsoAddr)
	require.Equal(t, cf.RedirectURL, client.CallbackAddr)
	require.Equal(t, cf.Attributes, client.Attributes)
	require.Equal(t, cf.DefaultRoles, client.DefaultRoles)
	require.True(t, client.CoverAttributes)
	require.Equal(t, cf.DisplayName, client.GetDisplayName())
	require.Equal(t, cf.SsoLogoutAddr, client.GetSsoLogoutAddr())
	transport := client.HTTPClient.Transport.(*http.Transport)
	t.Cleanup(transport.CloseIdleConnections)
	require.True(t, transport.TLSClientConfig.InsecureSkipVerify)
}

func TestReload_EnablesAndDisablesSSOMetadata(t *testing.T) {
	client := New(Config{})
	require.False(t, client.Enable)
	require.Empty(t, client.GetDisplayName())
	require.Empty(t, client.GetSsoLogoutAddr())
	cf := Config{Enable: true, DisplayName: "Updated", SsoLogoutAddr: "/new-logout", RedirectURL: "/callback"}
	cf.Attributes.Email = "email"

	client.Reload(cf)

	require.Equal(t, cf, client.Config)
	require.Equal(t, cf.Attributes, client.Attributes)
	require.Equal(t, "/callback", client.CallbackAddr)
	require.Equal(t, "Updated", client.GetDisplayName())
	require.Equal(t, "/new-logout", client.GetSsoLogoutAddr())
	client.Reload(Config{})
	require.False(t, client.Enable)
	require.Empty(t, client.GetDisplayName())
	require.Empty(t, client.GetSsoLogoutAddr())
}

func TestReload_EnablesTLSVerificationBypass(t *testing.T) {
	client := New(Config{Enable: true})

	client.Reload(Config{Enable: true, SkipTlsVerify: true})

	transport := client.HTTPClient.Transport.(*http.Transport)
	t.Cleanup(transport.CloseIdleConnections)
	require.Equal(t, &tls.Config{InsecureSkipVerify: true}, transport.TLSClientConfig)
}

func TestAuthorize_StoresExpiringRedirectAndBuildsLoginURL(t *testing.T) {
	server, redisClient := casRedis(t)
	tests := []struct {
		name      string
		addr      string
		loginPath string
		wantPath  string
	}{
		{name: "default", addr: "https://sso.example", wantPath: "/cas/login"},
		{name: "p3", addr: "https://sso.example/p3", wantPath: "/p3/login"},
		{name: "custom", addr: "https://sso.example/base?tenant=a", loginPath: "/signin", wantPath: "/base/signin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := New(Config{
				Enable: true, SsoAddr: tt.addr, LoginPath: tt.loginPath, RedirectURL: "https://n9e.example/cb?a=1",
			})

			location, state, err := client.Authorize(redisClient, "/dashboard?team=a")

			require.NoError(t, err)
			require.NotEmpty(t, state)
			stored, err := server.Get(wrapStateKey(state))
			require.NoError(t, err)
			require.Equal(t, "/dashboard?team=a", stored)
			require.Equal(t, 300*time.Second, server.TTL(wrapStateKey(state)))
			parsed, err := url.Parse(location)
			require.NoError(t, err)
			require.Equal(t, tt.wantPath, parsed.Path)
			require.Equal(t, "https://n9e.example/cb?a=1", parsed.Query().Get("service"))
			if tt.loginPath != "" {
				require.Equal(t, "a", parsed.Query().Get("tenant"))
			}
		})
	}
}

func TestAuthorize_ReturnsRedisFailure(t *testing.T) {
	server, redisClient := casRedis(t)
	server.SetError("ERR unavailable")
	client := New(Config{Enable: true})

	location, state, err := client.Authorize(redisClient, "/dashboard")

	require.ErrorContains(t, err, "unavailable")
	require.Empty(t, location)
	require.Empty(t, state)
}

func TestValidateServiceTicket_MapsAttributesAndConsumesRedirect(t *testing.T) {
	redisServer, redisClient := casRedis(t)
	require.NoError(t, redisServer.Set(wrapStateKey("state"), "/dashboard"))
	var method, path string
	var params url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, params = r.Method, r.URL.Path, r.URL.Query()
		_, _ = io.WriteString(w, `<cas:serviceResponse xmlns:cas="http://www.yale.edu/tp/cas">`+
			`<cas:authenticationSuccess><cas:user>alice</cas:user><cas:attributes>`+
			`<cas:displayName>Alice</cas:displayName><cas:mail>alice@example.test</cas:mail><cas:mobile>123</cas:mobile>`+
			`</cas:attributes></cas:authenticationSuccess></cas:serviceResponse>`)
	}))
	t.Cleanup(server.Close)
	cf := Config{Enable: true, SsoAddr: server.URL + "/cas", RedirectURL: "https://n9e.example/callback"}
	cf.Attributes.Nickname, cf.Attributes.Email, cf.Attributes.Phone = "displayName", "mail", "mobile"
	client := New(cf)
	client.HTTPClient = server.Client()

	output, err := client.ValidateServiceTicket(context.Background(), "ST-test", "state", redisClient)

	require.NoError(t, err)
	require.Equal(t, http.MethodGet, method)
	require.Equal(t, "/cas/serviceValidate", path)
	require.Equal(t, "ST-test", params.Get("ticket"))
	require.Equal(t, cf.RedirectURL, params.Get("service"))
	require.Equal(t, &CallbackOutput{
		Username: "alice", Nickname: "Alice", Email: "alice@example.test", Phone: "123", Redirect: "/dashboard",
	}, output)
	require.False(t, redisServer.Exists(wrapStateKey("state")))
}

func TestValidateServiceTicket_PreservesRedirectOnValidationFailure(t *testing.T) {
	redisServer, redisClient := casRedis(t)
	require.NoError(t, redisServer.Set(wrapStateKey("state"), "/dashboard"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `<cas:serviceResponse xmlns:cas="http://www.yale.edu/tp/cas">`+
			`<cas:authenticationFailure code="INVALID_TICKET">expired ticket</cas:authenticationFailure></cas:serviceResponse>`)
	}))
	t.Cleanup(server.Close)
	client := New(Config{Enable: true, SsoAddr: server.URL, RedirectURL: "https://n9e.example/cb"})

	output, err := client.ValidateServiceTicket(context.Background(), "expired", "state", redisClient)

	require.Error(t, err)
	require.Nil(t, output)
	require.True(t, redisServer.Exists(wrapStateKey("state")))
}

func TestValidateServiceTicket_RejectsMalformedURLs(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		callback string
	}{
		{name: "CAS URL", addr: "://invalid", callback: "https://n9e.example/cb"},
		{name: "callback URL", addr: "http://127.0.0.1", callback: "://invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := New(Config{Enable: true, SsoAddr: tt.addr, RedirectURL: tt.callback})

			output, err := client.ValidateServiceTicket(context.Background(), "ticket", "state", nil)

			require.Error(t, err)
			require.Nil(t, output)
		})
	}
}
