package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func feishuRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return mr, client
}

func feishuTestClient(t *testing.T, tokenBody, userBody string) (*SsoClient, <-chan *http.Request) {
	t.Helper()
	requests := make(chan *http.Request, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open-apis/auth/v3/app_access_token/internal", "/open-apis/auth/v3/tenant_access_token/internal":
			// An already-expired SDK token avoids retaining usable credentials between tests.
			fmt.Fprint(w, `{"code":0,"app_access_token":"app-test","tenant_access_token":"tenant-test","expire":0}`)
		case "/open-apis/authen/v1/access_token":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["code"] != "code-test" || body["grant_type"] != "authorization_code" {
				t.Errorf("unexpected token body: %v", body)
			}
			requests <- r.Clone(context.Background())
			fmt.Fprint(w, tokenBody)
		case "/open-apis/contact/v3/users/user-test":
			requests <- r.Clone(context.Background())
			fmt.Fprint(w, userBody)
		default:
			t.Errorf("unexpected request: %s", r.URL)
			http.Error(w, "unexpected request", 404)
		}
	}))
	t.Cleanup(server.Close)
	previousEndpoint := lark.FeishuBaseUrl
	t.Cleanup(func() { lark.FeishuBaseUrl = previousEndpoint })
	client := New(Config{
		Enable: true, AppID: t.Name(), AppSecret: "secret", FeiShuEndpoint: server.URL,
		RedirectURL: "http://localhost/callback", UsernameField: "email", DisplayName: "Company login",
	})
	return client, requests
}

const feishuValidToken = `{"code":0,"data":{"access_token":"access-test","user_id":"user-test"}}`

func TestAuthorize_EncodesURLAndStoresExpiringRedirect(t *testing.T) {
	mr, redisClient := feishuRedis(t)
	client := New(Config{Enable: true, AppID: "app-test", RedirectURL: "http://localhost/callback?a=1"})
	client.FeiShuConfig.AuthURL = "http://localhost/authorize?lang=en"

	location, err := client.Authorize(redisClient, "/dashboard")

	require.NoError(t, err)
	parsed, err := url.Parse(location)
	require.NoError(t, err)
	query := parsed.Query()
	require.Equal(t, "en", query.Get("lang"))
	require.Equal(t, "app-test", query.Get("app_id"))
	require.Equal(t, "http://localhost/callback?a=1", query.Get("redirect_uri"))
	state := query.Get("state")
	require.NotEmpty(t, state)
	mr.CheckGet(t, wrapStateKey(state), "/dashboard")
	require.Equal(t, 300*time.Second, mr.TTL(wrapStateKey(state)))
}

func TestAuthCodeURL_RequiresRedirect(t *testing.T) {
	client := New(Config{Enable: true})

	location, err := client.AuthCodeURL("state-test")

	require.Empty(t, location)
	require.ErrorContains(t, err, "RedirectURL is empty")
}

func TestReload_UpdatesDisplayNameAndDisabledState(t *testing.T) {
	client := New(Config{})
	require.Empty(t, client.GetDisplayName())

	client.Reload(Config{Enable: true, DisplayName: "Company login"})
	require.Equal(t, "Company login", client.GetDisplayName())
	client.Reload(Config{Enable: false, DisplayName: "Hidden"})

	require.False(t, client.Enable)
	require.Empty(t, client.GetDisplayName())
}

func TestFeishuMethods_RejectUninitializedSDK(t *testing.T) {
	client := &SsoClient{}

	_, _, tokenErr := client.GetUserToken("code-test")
	_, userErr := client.GetUserInfo("user-test")

	require.ErrorContains(t, tokenErr, "not initialized")
	require.ErrorContains(t, userErr, "not initialized")
}

func TestGetUserToken_RejectsInvalidAPIResponses(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{name: "api error", body: `{"code":123,"msg":"denied"}`, want: "code=123, msg=denied"},
		{name: "no data", body: `{"code":0}`, want: "empty data"},
		{name: "no user", body: `{"code":0,"data":{"access_token":"token"}}`, want: "empty user_id"},
		{name: "no token", body: `{"code":0,"data":{"user_id":"user-test"}}`, want: "empty access_token"},
		{name: "invalid json", body: `{`, want: "get access token error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := feishuTestClient(t, tc.body, "{}")

			token, userID, err := client.GetUserToken("code-test")

			require.ErrorContains(t, err, tc.want)
			require.Empty(t, token)
			require.Empty(t, userID)
		})
	}
}

func TestGetUserInfo_RejectsInvalidAPIResponses(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{name: "api error", body: `{"code":123,"msg":"denied"}`, want: "code=123, msg=denied"},
		{name: "no data", body: `{"code":0}`, want: "empty user data"},
		{name: "no user", body: `{"code":0,"data":{}}`, want: "empty user data"},
		{name: "invalid json", body: `{`, want: "get user detail error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := feishuTestClient(t, feishuValidToken, tc.body)

			user, err := client.GetUserInfo("user-test")

			require.Nil(t, user)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestCallback_MapsConfiguredUsernameAndConsumesState(t *testing.T) {
	cases := []struct {
		field, username string
	}{
		{field: "userid", username: "user-test"},
		{field: "name", username: "Alice"},
		{field: "phone", username: "123"},
		{field: "email", username: "alice@example.test"},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			user := `{"code":0,"data":{"user":{"user_id":"user-test","name":"Alice",`
			user += `"mobile":"123","enterprise_email":"alice@example.test"}}}`
			client, requests := feishuTestClient(t, feishuValidToken, user)
			client.FeiShuConfig.UsernameField = tc.field
			mr, redisClient := feishuRedis(t)
			mr.Set(wrapStateKey("state-test"), "/dashboard")

			output, err := client.Callback(redisClient, context.Background(), "code-test", "state-test")

			require.NoError(t, err)
			require.Equal(t, &CallbackOutput{
				Redirect: "/dashboard", AccessToken: "access-test", Username: tc.username,
				Nickname: "Alice", Phone: "123", Email: "alice@example.test",
			}, output)
			require.False(t, mr.Exists(wrapStateKey("state-test")))
			tokenRequest, userRequest := <-requests, <-requests
			require.Equal(t, "POST", tokenRequest.Method)
			require.Equal(t, "Bearer app-test", tokenRequest.Header.Get("Authorization"))
			require.Equal(t, "GET", userRequest.Method)
			require.Equal(t, "user_id", userRequest.URL.Query().Get("user_id_type"))
			require.Equal(t, "Bearer tenant-test", userRequest.Header.Get("Authorization"))
		})
	}
}

func TestCallback_RejectsMissingUsernameAttribute(t *testing.T) {
	for _, field := range []string{"userid", "name", "phone", "email"} {
		t.Run(field, func(t *testing.T) {
			userBody := `{"code":0,"data":{"user":{"user_id":"user-test"}}}`
			want := "feishu user " + field + " is empty"
			if field == "userid" {
				userBody = `{"code":0,"data":{"user":{}}}`
				want = "feishu user_id is empty"
			}
			client, _ := feishuTestClient(t, feishuValidToken, userBody)
			client.FeiShuConfig.UsernameField = field
			_, redisClient := feishuRedis(t)

			output, err := client.Callback(redisClient, context.Background(), "code-test", "missing-state")

			require.Nil(t, output)
			require.ErrorContains(t, err, want)
		})
	}
}

func TestCallback_DefaultsMissingRedirectToRoot(t *testing.T) {
	userBody := `{"code":0,"data":{"user":{"user_id":"user-test","email":"alice@example.test"}}}`
	client, _ := feishuTestClient(t, feishuValidToken, userBody)
	_, redisClient := feishuRedis(t)

	output, err := client.Callback(redisClient, context.Background(), "code-test", "missing-state")

	require.NoError(t, err)
	require.Equal(t, "/", output.Redirect)
	require.Equal(t, "alice@example.test", output.Username)
}
