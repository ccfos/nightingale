package dingtalk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func localDingTalkRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return server, client
}

func TestAuthCodeURL_EncodesRedirectStateAndOrganization(t *testing.T) {
	tests := []struct {
		name    string
		authURL string
		corpID  string
		scope   string
	}{
		{name: "default authorization", scope: "openid"},
		{name: "organization", authURL: "https://login.local/auth?tenant=one", corpID: "corp-1", scope: "openid corpid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := New(Config{
				Enable: true, ClientID: "app-1", RedirectURL: "https://app.local/callback?next=/alerts",
				AuthURL: tt.authURL, CorpId: tt.corpID,
			})

			target, err := client.AuthCodeURL("state with & symbols")

			require.NoError(t, err)
			parsed, err := url.Parse(target)
			require.NoError(t, err)
			params := parsed.Query()
			require.Equal(t, "app-1", params.Get("client_id"))
			require.Equal(t, "code", params.Get("response_type"))
			require.Equal(t, "consent", params.Get("prompt"))
			require.Equal(t, "https://app.local/callback?next=/alerts", params.Get("redirect_uri"))
			require.Equal(t, "state with & symbols", params.Get("state"))
			require.Equal(t, tt.scope, params.Get("scope"))
			require.Equal(t, tt.corpID, params.Get("corpId"))
			if tt.authURL != "" {
				require.Equal(t, "one", params.Get("tenant"))
			} else {
				require.Equal(t, "login.dingtalk.com", parsed.Host)
			}
		})
	}
}

func TestAuthCodeURL_RejectsEmptyRedirect(t *testing.T) {
	client := New(Config{Enable: true})

	target, err := client.AuthCodeURL("state")

	require.Empty(t, target)
	require.ErrorContains(t, err, "RedirectURL is empty")
}

func TestReload_UpdatesEnabledDisplayName(t *testing.T) {
	client := New(Config{})
	require.Empty(t, client.GetDisplayName())

	client.Reload(Config{Enable: true, DisplayName: "Company SSO"})

	require.True(t, client.Enable)
	require.Equal(t, "Company SSO", client.GetDisplayName())
}

func TestSetEndpoint_SeparatesProtocolAndHost(t *testing.T) {
	tests := []struct {
		endpoint string
		protocol string
		host     string
	}{
		{endpoint: "http://127.0.0.1:1234", protocol: "http", host: "127.0.0.1:1234"},
		{endpoint: "https://login.local", protocol: "https", host: "login.local"},
		{endpoint: "login.local", protocol: "https", host: "login.local"},
	}
	for _, tt := range tests {
		t.Run(tt.endpoint, func(t *testing.T) {
			config := &openapi.Config{}

			err := (&Config{}).setEndpoint(config, tt.endpoint)

			require.NoError(t, err)
			require.Equal(t, tt.protocol, tea.StringValue(config.Protocol))
			require.Equal(t, tt.host, tea.StringValue(config.Endpoint))
		})
	}
}

func TestSetProxy_ChoosesHTTPOrHTTPSProxy(t *testing.T) {
	for _, proxy := range []string{"http://127.0.0.1:8888", "https://127.0.0.1:8888"} {
		t.Run(proxy, func(t *testing.T) {
			config := &openapi.Config{}

			err := (&Config{Proxy: proxy}).setProxy(config)

			require.NoError(t, err)
			if proxy[:5] == "https" {
				require.Equal(t, proxy, tea.StringValue(config.HttpsProxy))
			} else {
				require.Equal(t, proxy, tea.StringValue(config.HttpProxy))
			}
		})
	}
}

func TestAuthorize_CachesRedirectForGeneratedState(t *testing.T) {
	server, redisClient := localDingTalkRedis(t)
	client := New(Config{Enable: true, RedirectURL: "https://app.local/callback"})

	target, err := client.Authorize(redisClient, "/alerts?team=ops")

	require.NoError(t, err)
	parsed, err := url.Parse(target)
	require.NoError(t, err)
	state := parsed.Query().Get("state")
	require.NotEmpty(t, state)
	redirect, err := fetchRedirect(redisClient, context.Background(), state)
	require.NoError(t, err)
	require.Equal(t, "/alerts?team=ops", redirect)
	require.Equal(t, 5*time.Minute, server.TTL(wrapStateKey(state)))
}

func TestAccessTokenCache_ExpiresBeforeProviderToken(t *testing.T) {
	server, redisClient := localDingTalkRedis(t)
	c := context.Background()

	err := dingTalkAccessTokenCacheSet(redisClient, c, "cached-token")

	require.NoError(t, err)
	token, err := dingTalkAccessTokenCacheGet(redisClient, c)
	require.NoError(t, err)
	require.Equal(t, "cached-token", token)
	require.Equal(t, 90*time.Minute, server.TTL(wrapStateKey("dingtalk_access_token")))
	server.FastForward(90 * time.Minute)
	_, err = dingTalkAccessTokenCacheGet(redisClient, c)
	require.ErrorIs(t, err, redis.Nil)
}

func TestGetUserToken_PostsAuthorizationCodeAndParsesToken(t *testing.T) {
	var path, method string
	var body map[string]interface{}
	var decodeErr error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, method = r.URL.Path, r.Method
		decodeErr = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"accessToken":"user-token","expireIn":7200}`)
	}))
	defer server.Close()
	client := New(Config{Enable: true, ClientID: "app-1", ClientSecret: "secret", Endpoint: server.URL})

	token, err := client.GetUserToken("code-1")

	require.NoError(t, err)
	require.NoError(t, decodeErr)
	require.Equal(t, "user-token", token)
	require.Equal(t, "/v1.0/oauth2/userAccessToken", path)
	require.Equal(t, http.MethodPost, method)
	require.Equal(t, "app-1", body["clientId"])
	require.Equal(t, "secret", body["clientSecret"])
	require.Equal(t, "code-1", body["code"])
	require.Equal(t, "authorization_code", body["grantType"])
}

func TestTokenRequests_ReportInvalidEndpoint(t *testing.T) {
	client := New(Config{Enable: true, ClientID: "app-1", ClientSecret: "secret", Endpoint: "http://["})

	_, userErr := client.GetUserToken("code-1")
	_, appErr := client.GetAccessToken()

	// Before the fix both calls dereferenced a nil client and panicked.
	require.ErrorContains(t, userErr, "create client")
	require.ErrorContains(t, appErr, "create client")
}

func TestGetAccessToken_PostsAppCredentials(t *testing.T) {
	var path string
	var body map[string]interface{}
	var decodeErr error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		decodeErr = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"accessToken":"app-token","expireIn":7200}`)
	}))
	defer server.Close()
	client := New(Config{Enable: true, ClientID: "app-1", ClientSecret: "secret", Endpoint: server.URL})

	token, err := client.GetAccessToken()

	require.NoError(t, err)
	require.NoError(t, decodeErr)
	require.Equal(t, "app-token", token)
	require.Equal(t, "/v1.0/oauth2/accessToken", path)
	require.Equal(t, map[string]interface{}{"appKey": "app-1", "appSecret": "secret"}, body)
}

func TestGetUserToken_ReturnsProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":"InvalidCode","message":"authorization expired"}`)
	}))
	defer server.Close()
	client := New(Config{Enable: true, Endpoint: server.URL})

	token, err := client.GetUserToken("expired")

	require.Empty(t, token)
	require.ErrorContains(t, err, "dingTalk sso get token error")
	require.ErrorContains(t, err, "InvalidCode")
}

func TestCallback_MapsSelectedUsernameAndConsumesRedirect(t *testing.T) {
	tests := []struct {
		field    string
		username string
	}{
		{field: "name", username: "Alice"},
		{field: "email", username: "alice@example.local"},
		{field: "mobile", username: "123456"},
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			_, redisClient := localDingTalkRedis(t)
			var contactAuth string
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/v1.0/oauth2/userAccessToken" {
					_, _ = io.WriteString(w, `{"accessToken":"user-token"}`)
					return
				}
				contactAuth = r.Header.Get("x-acs-dingtalk-access-token")
				_, _ = io.WriteString(w, `{"nick":"Alice","mobile":"123456","email":"alice@example.local","unionId":"union-1"}`)
			}))
			defer server.Close()
			client := New(Config{Enable: true, Endpoint: server.URL, UsernameField: tt.field})
			c := context.Background()
			require.NoError(t, redisClient.Set(c, wrapStateKey("state-1"), "/dashboard", time.Minute).Err())

			result, err := client.Callback(redisClient, c, "code-1", "state-1")

			require.NoError(t, err)
			require.Equal(t, &CallbackOutput{
				Redirect: "/dashboard", Username: tt.username, Nickname: "Alice", Phone: "123456", Email: "alice@example.local",
			}, result)
			require.Equal(t, "user-token", contactAuth)
			require.Equal(t, []string{"/v1.0/oauth2/userAccessToken", "/v1.0/contact/users/me"}, paths)
			require.False(t, redisClient.Exists(c, wrapStateKey("state-1")).Val() > 0)
		})
	}
}

func TestCallback_RejectsMissingSelectedUsername(t *testing.T) {
	for _, field := range []string{"name", "email", "mobile"} {
		t.Run(field, func(t *testing.T) {
			_, redisClient := localDingTalkRedis(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/v1.0/oauth2/userAccessToken" {
					_, _ = io.WriteString(w, `{"accessToken":"user-token"}`)
				} else {
					_, _ = io.WriteString(w, `{}`)
				}
			}))
			defer server.Close()
			client := New(Config{Enable: true, Endpoint: server.URL, UsernameField: field})

			result, err := client.Callback(redisClient, context.Background(), "code-1", "absent-state")

			require.Nil(t, result)
			require.ErrorContains(t, err, "is empty")
		})
	}
}
