package oidcx

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	oidc "github.com/coreos/go-oidc"
	"github.com/golang-jwt/jwt"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func loginRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return mr, client
}

func loginProvider(t *testing.T, tokenBody func(string, *rsa.PrivateKey) string, userInfo string) (*SsoClient, <-chan url.Values) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	requests := make(chan url.Values, 4)
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer": server.URL, "authorization_endpoint": server.URL + "/authorize",
			"token_endpoint": server.URL + "/token", "jwks_uri": server.URL + "/keys",
			"userinfo_endpoint": server.URL + "/userinfo",
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
		e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
		fmt.Fprintf(w, `{"keys":[{"kty":"RSA","alg":"RS256","n":%q,"e":%q}]}`, n, e)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		requests <- r.PostForm
		user, pass, ok := r.BasicAuth()
		if !ok || user != "n9e" || pass != "secret" {
			t.Error("missing OAuth client credentials")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, tokenBody(server.URL, key))
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-test" {
			t.Error("missing userinfo bearer token")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, userInfo)
	})
	config := Config{
		Enable: true, SsoAddr: server.URL, RedirectURL: "http://localhost/callback",
		ClientId: "n9e", ClientSecret: "secret", DisplayName: "Company login",
		SsoLogoutAddr: server.URL + "/logout?id={{$__id_token__}}", DefaultTeams: []int64{3},
	}
	config.Attributes.Nickname = "nickname"
	config.Attributes.Phone = "phone"
	config.Attributes.Email = "email"
	client, err := New(config)
	require.NoError(t, err)
	// Keep JWT expiry validation independent of the machine's clock.
	client.Verifier = client.Provider.Verifier(&oidc.Config{
		ClientID: "n9e", Now: func() time.Time { return time.Unix(1700000000, 0) },
	})
	return client, requests
}

func loginToken(t *testing.T, issuer string, key *rsa.PrivateKey, attributes jwt.MapClaims) string {
	t.Helper()
	claims := jwt.MapClaims{"iss": issuer, "aud": "n9e", "sub": "alice", "exp": int64(1800000000)}
	for name, value := range attributes {
		claims[name] = value
	}
	idToken := signRS256(t, key, claims)
	body, err := json.Marshal(map[string]string{
		"access_token": "access-test", "token_type": "Bearer", "id_token": idToken,
	})
	require.NoError(t, err)
	return string(body)
}

func TestAuthorize_PersistsRedirectAndBuildsOIDCURL(t *testing.T) {
	client, _ := loginProvider(t, func(string, *rsa.PrivateKey) string { return "{}" }, "{}")
	mr, redisClient := loginRedis(t)

	location, err := client.Authorize(redisClient, "/dashboard?team=3")

	require.NoError(t, err)
	parsed, err := url.Parse(location)
	require.NoError(t, err)
	query := parsed.Query()
	require.Equal(t, "/authorize", parsed.Path)
	require.Equal(t, "n9e", query.Get("client_id"))
	require.Equal(t, "code", query.Get("response_type"))
	require.Equal(t, "http://localhost/callback", query.Get("redirect_uri"))
	require.Equal(t, "openid profile email phone", query.Get("scope"))
	state := query.Get("state")
	require.NotEmpty(t, state)
	mr.CheckGet(t, wrapStateKey(state), "/dashboard?team=3")
	require.Equal(t, 300*time.Second, mr.TTL(wrapStateKey(state)))
}

func TestAuthorize_RejectsUninitializedProvider(t *testing.T) {
	location, err := (&SsoClient{}).Authorize(nil, "/")

	require.Empty(t, location)
	require.ErrorContains(t, err, "provider is not initialized")
}

func TestCallback_MapsClaimsAndConsumesRedirect(t *testing.T) {
	client, requests := loginProvider(t, func(issuer string, key *rsa.PrivateKey) string {
		return loginToken(t, issuer, key, jwt.MapClaims{"nickname": "Alice", "email": "alice@example.test", "phone": "123"})
	}, `{"sub":"alice","email":"other@example.test","nickname":"Other","phone":"456"}`)
	mr, redisClient := loginRedis(t)
	mr.Set(wrapStateKey("state-test"), "/dashboard")

	output, err := client.Callback(redisClient, context.Background(), "code-test", "state-test")

	require.NoError(t, err)
	require.Equal(t, "alice", output.Username)
	require.Equal(t, "Alice", output.Nickname)
	require.Equal(t, "alice@example.test", output.Email)
	require.Equal(t, "123", output.Phone)
	require.Equal(t, "/dashboard", output.Redirect)
	require.Equal(t, "access-test", output.AccessToken)
	require.NotEmpty(t, output.IdToken)
	require.False(t, mr.Exists(wrapStateKey("state-test")))
	form := <-requests
	require.Equal(t, "code-test", form.Get("code"))
	require.Equal(t, "authorization_code", form.Get("grant_type"))
	require.Equal(t, "http://localhost/callback", form.Get("redirect_uri"))
}

func TestExchangeUser_FillsMissingClaimsFromUserInfo(t *testing.T) {
	client, _ := loginProvider(t, func(issuer string, key *rsa.PrivateKey) string {
		return loginToken(t, issuer, key, nil)
	}, `{"sub":"alice","email":"alice@example.test","nickname":"Alice","phone":"123"}`)

	output, err := client.exchangeUser("code-test")

	require.NoError(t, err)
	require.Equal(t, "Alice", output.Nickname)
	require.Equal(t, "alice@example.test", output.Email)
	require.Equal(t, "123", output.Phone)
}

func TestExchangeUser_KeepsIDClaimsWhenUserInfoFails(t *testing.T) {
	client, _ := loginProvider(t, func(issuer string, key *rsa.PrivateKey) string {
		return loginToken(t, issuer, key, jwt.MapClaims{"nickname": "Alice"})
	}, `invalid json`)

	output, err := client.exchangeUser("code-test")

	require.NoError(t, err)
	require.Equal(t, "alice", output.Username)
	require.Equal(t, "Alice", output.Nickname)
}

func TestExchangeUser_RejectsMissingOrInvalidIDToken(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{name: "missing", body: `{"access_token":"access-test","token_type":"Bearer"}`, want: "no id_token field"},
		{name: "invalid", body: `{"access_token":"access-test","id_token":"invalid"}`, want: "failed to verify id_token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := loginProvider(t, func(string, *rsa.PrivateKey) string { return tc.body }, "{}")

			output, err := client.exchangeUser("code-test")

			require.Nil(t, output)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestReload_DisablesLoginAndLogout(t *testing.T) {
	client, _ := loginProvider(t, func(string, *rsa.PrivateKey) string { return "{}" }, "{}")
	require.Equal(t, "Company login", client.GetDisplayName())
	require.Equal(t, client.SsoAddr+"/logout?id=token-test", client.GetSsoLogoutAddr("token-test"))
	require.Equal(t, client.SsoLogoutAddr, client.GetSsoLogoutAddr(""))
	require.Equal(t, []int64{3}, client.DefaultTeams)

	err := client.Reload(Config{Enable: false})

	require.NoError(t, err)
	require.Empty(t, client.GetDisplayName())
	require.Empty(t, client.GetSsoLogoutAddr("token-test"))
}
