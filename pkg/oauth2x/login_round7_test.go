package oauth2x

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func loginRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return server, client
}

func TestAuthorize_PersistsRedirectWithExpiringRandomState(t *testing.T) {
	server, redisClient := loginRedis(t)
	client := New(Config{
		Enable: true, SsoAddr: "http://127.0.0.1/authorize", ClientId: "n9e",
		RedirectURL: "http://127.0.0.1/callback", Scopes: []string{"profile", "email"},
	})

	location, err := client.Authorize(redisClient, "/dashboards?team=ops")

	require.NoError(t, err)
	parsed, err := url.Parse(location)
	require.NoError(t, err)
	require.Equal(t, "/authorize", parsed.Path)
	query := parsed.Query()
	require.Equal(t, "code", query.Get("response_type"))
	require.Equal(t, "n9e", query.Get("client_id"))
	require.Equal(t, "http://127.0.0.1/callback", query.Get("redirect_uri"))
	require.Equal(t, "profile email", query.Get("scope"))
	state := query.Get("state")
	require.NotEmpty(t, state)
	redirect, err := server.Get("n9e_oauth_" + state)
	require.NoError(t, err)
	require.Equal(t, "/dashboards?team=ops", redirect)
	require.Equal(t, 300*time.Second, server.TTL("n9e_oauth_"+state))
}

func TestAuthorize_ReturnsRedisErrorWithoutLocation(t *testing.T) {
	server, redisClient := loginRedis(t)
	server.SetError("ERR storage unavailable")
	client := New(Config{Enable: true})

	location, err := client.Authorize(redisClient, "/dashboard")

	require.ErrorContains(t, err, "storage unavailable")
	require.Empty(t, location)
}

func TestCallback_RestoresRedirectAndConsumesState(t *testing.T) {
	server, redisClient := loginRedis(t)
	require.NoError(t, server.Set("n9e_oauth_state-1", "/dashboard/42"))
	fake := &fake4A{t: t}
	provider := httptest.NewServer(fake.handler())
	t.Cleanup(provider.Close)
	client := new4AClient(provider.URL)

	output, err := client.Callback(redisClient, context.Background(), "code-1", "state-1")

	require.NoError(t, err)
	require.Equal(t, "/dashboard/42", output.Redirect)
	require.Equal(t, "user1", output.Username)
	require.Equal(t, "at-1", output.AccessToken)
	require.False(t, server.Exists("n9e_oauth_state-1"))
}

func TestCallback_ExchangeFailurePreservesState(t *testing.T) {
	server, redisClient := loginRedis(t)
	require.NoError(t, server.Set("n9e_oauth_state-1", "/dashboard/42"))
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	t.Cleanup(provider.Close)
	client := new4AClient(provider.URL)

	output, err := client.Callback(redisClient, context.Background(), "bad-code", "state-1")

	require.ErrorContains(t, err, "illegal user")
	require.ErrorContains(t, err, "invalid_grant")
	require.Nil(t, output)
	require.True(t, server.Exists("n9e_oauth_state-1"))
}

func TestReload_DisablesLoginMetadata(t *testing.T) {
	client := New(Config{
		Enable: true, DisplayName: "Company login", SsoLogoutAddr: "http://127.0.0.1/logout", DefaultTeams: []int64{3, 7},
	})
	require.Equal(t, "Company login", client.GetDisplayName())
	require.Equal(t, "http://127.0.0.1/logout", client.GetSsoLogoutAddr())
	require.Equal(t, []int64{3, 7}, client.GetDefaultTeams())

	client.Reload(Config{Enable: false})

	require.Empty(t, client.GetDisplayName())
	require.Empty(t, client.GetSsoLogoutAddr())
	require.Nil(t, client.GetDefaultTeams())
}

func TestUserInfo_QueryStringTransportSendsTokenAndClientID(t *testing.T) {
	type request struct {
		query         url.Values
		authorization string
	}
	requests := make(chan request, 1)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- request{query: r.URL.Query(), authorization: r.Header.Get("Authorization")}
		_, _ = w.Write([]byte(`{"sub":"alice"}`))
	}))
	t.Cleanup(provider.Close)
	client := New(Config{Enable: true})

	body, err := client.getUserInfo("n9e", provider.URL, "token-123", "querystring")

	require.NoError(t, err)
	require.JSONEq(t, `{"sub":"alice"}`, string(body))
	got := <-requests
	require.Equal(t, url.Values{"access_token": {"token-123"}, "client_id": {"n9e"}}, got.query)
	require.Equal(t, "Bearer token-123", got.authorization)
}
