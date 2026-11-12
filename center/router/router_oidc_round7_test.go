package router

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/oidcx"
	"github.com/coreos/go-oidc"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

// The verifier still checks issuer and audience; the fake key set supplies trusted test claims.
type oidcRound7KeySet struct{ payload []byte }

func (f oidcRound7KeySet) VerifySignature(context.Context, string) ([]byte, error) {
	return f.payload, nil
}

func oidcRound7Router(t *testing.T) (*Router, string) {
	t.Helper()
	rt, redis := sessionRouter(t)
	require.NoError(t, rt.Ctx.DB.AutoMigrate(&models.UserGroup{}, &models.UserGroupMember{}))
	payload, err := json.Marshal(map[string]interface{}{
		"iss": "http://127.0.0.1/test-issuer", "aud": "client", "sub": "subject-1",
		"login": "alice", "name": "OIDC Alice", "phone": "12345", "email": "alice@example.invalid",
	})
	require.NoError(t, err)
	encode := base64.RawURLEncoding.EncodeToString
	idToken := encode([]byte(`{"alg":"RS256"}`)) + "." + encode(payload) + "." + encode([]byte("test-signature"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			endpoint := "http://" + r.Host
			_ = json.NewEncoder(w).Encode(map[string]string{
				"issuer": endpoint, "authorization_endpoint": endpoint + "/authorize", "token_endpoint": endpoint + "/token",
				"jwks_uri": endpoint + "/keys", "userinfo_endpoint": endpoint + "/userinfo",
			})
			return
		case "/userinfo":
			_, _ = io.WriteString(w, `{"sub":"subject-1"}`)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("code") != "valid-code" {
			t.Errorf("unexpected code: %v", r.Form)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{
			"access_token": "provider-access", "token_type": "Bearer", "id_token": idToken,
		}); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	provider, err := oidc.NewProvider(context.Background(), server.URL)
	require.NoError(t, err)
	cli := &oidcx.SsoClient{
		Enable: true, Provider: provider, Ctx: context.Background(),
		DefaultRoles: []string{"Standard"}, DefaultTeams: []int64{10},
		Config: oauth2.Config{
			ClientID: "client",
			Endpoint: oauth2.Endpoint{TokenURL: server.URL + "/token", AuthStyle: oauth2.AuthStyleInParams},
		},
		// Expiry is irrelevant to router behavior and intentionally disabled for this fixed fixture.
		Verifier: oidc.NewVerifier("http://127.0.0.1/test-issuer", oidcRound7KeySet{payload: payload},
			&oidc.Config{ClientID: "client", SkipExpiryCheck: true}),
	}
	cli.Attributes.Username, cli.Attributes.Nickname = "login", "name"
	cli.Attributes.Phone, cli.Attributes.Email = "phone", "email"
	rt.Sso.OIDC = cli
	require.NoError(t, redis.Set("n9e_oidc_state", "/boards"))
	return rt, idToken
}

func TestLoginCallbackOIDC_CreatesProfileDefaultTeamAndStoresIdentityToken(t *testing.T) {
	rt, idToken := oidcRound7Router(t)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroup{Id: 10, Name: "oncall", UpdateAt: 1}).Error)

	w := identityRequest(t, rt, rt.loginCallback, nil, "0?code=valid-code&state=state", nil)

	var output CallbackOutput
	require.Empty(t, identityReply(t, w, &output))
	require.Equal(t, "/boards", output.Redirect)
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved).Error)
	require.Equal(t, "alice", saved.Username)
	require.Equal(t, "OIDC Alice", saved.Nickname)
	require.Equal(t, "12345", saved.Phone)
	require.Equal(t, "alice@example.invalid", saved.Email)
	require.Equal(t, "oidc", saved.Belong)
	require.Equal(t, "Standard", saved.Roles)
	var members []models.UserGroupMember
	require.NoError(t, rt.Ctx.DB.Find(&members).Error)
	require.Equal(t, []models.UserGroupMember{{GroupId: 10, UserId: saved.Id}}, members)
	requireSSORound7Session(t, rt, output, saved.Id)
	token, err := rt.fetchIdToken(context.Background(), saved.Id)
	require.NoError(t, err)
	require.Equal(t, idToken, token)
	require.Zero(t, rt.Redis.Exists(context.Background(), "n9e_oidc_state").Val())
}

func TestLoginCallbackOIDC_CoversProfileWhilePreservingLocalCredentials(t *testing.T) {
	rt, _ := oidcRound7Router(t)
	rt.Sso.OIDC.CoverAttributes = true
	original := models.User{Id: 7, Username: "alice", Nickname: "Local", Roles: "Viewer", Password: "local-hash"}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)
	require.NoError(t, rt.Redis.Set(context.Background(), "n9e_oidc_state", "/login", 0).Err())

	w := identityRequest(t, rt, rt.loginCallback, nil, "0?code=valid-code&state=state", nil)

	var output CallbackOutput
	require.Empty(t, identityReply(t, w, &output))
	require.Equal(t, "/", output.Redirect)
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved, 7).Error)
	require.Equal(t, "OIDC Alice", saved.Nickname)
	require.Equal(t, "oidc", saved.UpdateBy)
	require.Equal(t, original.Roles, saved.Roles)
	require.Equal(t, original.Password, saved.Password)
	requireSSORound7Session(t, rt, output, 7)
}

func TestLoginCallbackOIDC_DisabledUserDoesNotGetTokens(t *testing.T) {
	rt, _ := oidcRound7Router(t)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Username: "alice", Disabled: 1}).Error)

	w := identityRequest(t, rt, rt.loginCallback, nil, "0?code=valid-code&state=state", nil)

	require.Contains(t, identityReply(t, w, nil), models.ErrUserDisabled)
	keys, err := rt.Redis.Keys(context.Background(), "*").Result()
	require.NoError(t, err)
	require.Empty(t, keys)
}

func TestLoginCallbackOIDC_ExchangeFailureDoesNotCreateUser(t *testing.T) {
	rt, redis := sessionRouter(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
	}))
	t.Cleanup(server.Close)
	rt.Sso.OIDC = &oidcx.SsoClient{Ctx: context.Background(), Config: oauth2.Config{
		Endpoint: oauth2.Endpoint{TokenURL: server.URL, AuthStyle: oauth2.AuthStyleInParams},
	}}

	w := identityRequest(t, rt, rt.loginCallback, nil, "0?code=expired&state=state", nil)

	require.Contains(t, identityReply(t, w, nil), "invalid_grant")
	require.Empty(t, redis.Keys())
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.User{}).Count(&count).Error)
	require.Zero(t, count)
}
