package router

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/cas"
	"github.com/ccfos/nightingale/v6/pkg/dingtalk"
	"github.com/ccfos/nightingale/v6/pkg/feishu"
	"github.com/ccfos/nightingale/v6/pkg/oauth2x"
	"github.com/ccfos/nightingale/v6/pkg/oidcx"
	"github.com/coreos/go-oidc"
	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func ssoRound7Clients(rt *Router) {
	rt.Sso.OIDC = &oidcx.SsoClient{}
	rt.Sso.CAS = cas.New(cas.Config{})
	rt.Sso.OAuth2 = oauth2x.New(oauth2x.Config{})
	rt.Sso.DingTalk = dingtalk.New(dingtalk.Config{})
	rt.Sso.FeiShu = feishu.New(feishu.Config{})
}

func ssoRound7Redirects(rt *Router) []struct {
	name    string
	handler gin.HandlerFunc
} {
	return []struct {
		name    string
		handler gin.HandlerFunc
	}{
		{name: "oidc", handler: rt.loginRedirect},
		{name: "cas", handler: rt.loginRedirectCas},
		{name: "oauth", handler: rt.loginRedirectOAuth},
		{name: "dingtalk", handler: rt.loginRedirectDingTalk},
		{name: "feishu", handler: rt.loginRedirectFeiShu},
	}
}

func TestSSORedirect_DisabledProviderReturnsEmptyLocation(t *testing.T) {
	rt, redis := sessionRouter(t)
	ssoRound7Clients(rt)
	for _, tc := range ssoRound7Redirects(rt) {
		t.Run(tc.name, func(t *testing.T) {
			w := identityRequest(t, rt, tc.handler, nil, "0?redirect=%2Fboards", nil)

			var location string
			require.Empty(t, identityReply(t, w, &location))
			require.Empty(t, location)
			require.Empty(t, redis.Keys())
		})
	}
}

func TestSSORedirect_AuthenticatedUserKeepsRequestedLocation(t *testing.T) {
	rt, redis := sessionRouter(t)
	ssoRound7Clients(rt)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 7, Username: "alice"}).Error)
	for _, tc := range ssoRound7Redirects(rt) {
		t.Run(tc.name, func(t *testing.T) {
			handler := func(c *gin.Context) { c.Set("userid", int64(7)); tc.handler(c) }

			w := identityRequest(t, rt, handler, nil, "0?redirect=%2Fboards%3Fteam%3Dops", nil)

			var location string
			require.Empty(t, identityReply(t, w, &location))
			require.Equal(t, "/boards?team=ops", location)
			require.Empty(t, redis.Keys())
		})
	}
}

func TestSSORedirect_MissingAuthenticatedUserDoesNotIssueState(t *testing.T) {
	rt, redis := sessionRouter(t)
	ssoRound7Clients(rt)
	for _, tc := range ssoRound7Redirects(rt) {
		t.Run(tc.name, func(t *testing.T) {
			handler := func(c *gin.Context) { c.Set("userid", int64(404)); tc.handler(c) }

			w := identityRequest(t, rt, handler, nil, "0", nil)

			require.Contains(t, identityReply(t, w, nil), "user not found")
			require.Empty(t, redis.Keys())
		})
	}
}

func TestSSORedirect_EnabledProviderStoresExpiringReturnLocation(t *testing.T) {
	cases := []struct {
		name   string
		prefix string
	}{
		{name: "oidc", prefix: "n9e_oidc_"},
		{name: "cas", prefix: "n9e_cas_"},
		{name: "oauth", prefix: "n9e_oauth_"},
		{name: "dingtalk", prefix: "n9e_dingtalk_oauth_"},
		{name: "feishu", prefix: "n9e_feishu_oauth_"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, redis := sessionRouter(t)
			ssoRound7Clients(rt)
			cf := oauth2x.Config{
				Enable: true, SsoAddr: "http://127.0.0.1/authorize", RedirectURL: "http://127.0.0.1/callback", ClientId: "client",
			}
			rt.Sso.OAuth2 = oauth2x.New(cf)
			rt.Sso.OIDC.Enable = true
			rt.Sso.OIDC.Provider = &oidc.Provider{}
			rt.Sso.OIDC.Config = oauth2.Config{
				ClientID: "client", Endpoint: oauth2.Endpoint{AuthURL: cf.SsoAddr}, RedirectURL: cf.RedirectURL,
			}
			rt.Sso.CAS = cas.New(cas.Config{Enable: true, SsoAddr: "http://127.0.0.1", RedirectURL: cf.RedirectURL})
			rt.Sso.DingTalk = dingtalk.New(dingtalk.Config{Enable: true, ClientID: "client", RedirectURL: cf.RedirectURL})
			rt.Sso.FeiShu = feishu.New(feishu.Config{Enable: true, AppID: "client", RedirectURL: cf.RedirectURL})
			var handler gin.HandlerFunc
			for _, entry := range ssoRound7Redirects(rt) {
				if entry.name == tc.name {
					handler = entry.handler
				}
			}

			w := identityRequest(t, rt, handler, nil, "0?redirect=%2Fboards%3Fteam%3Dops", nil)

			var location, state string
			if tc.name == "cas" {
				var output RedirectOutput
				require.Empty(t, identityReply(t, w, &output))
				location, state = output.Redirect, output.State
			} else {
				require.Empty(t, identityReply(t, w, &location))
			}
			parsed, err := url.Parse(location)
			require.NoError(t, err)
			if state == "" {
				state = parsed.Query().Get("state")
			}
			require.NotEmpty(t, state)
			saved, err := redis.Get(tc.prefix + state)
			require.NoError(t, err)
			require.Equal(t, "/boards?team=ops", saved)
			require.Equal(t, 300*time.Second, redis.TTL(tc.prefix+state))
			if tc.name == "cas" {
				require.Equal(t, "/cas/login", parsed.Path)
				require.Equal(t, cf.RedirectURL, parsed.Query().Get("service"))
			} else {
				require.Equal(t, cf.RedirectURL, parsed.Query().Get("redirect_uri"))
			}
		})
	}
}

func oauthRound7Router(t *testing.T, userinfo string) *Router {
	t.Helper()
	rt, redis := sessionRouter(t)
	require.NoError(t, rt.Ctx.DB.AutoMigrate(&models.UserGroup{}, &models.UserGroupMember{}))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.Form.Get("code") != "valid-code" {
				t.Errorf("unexpected code: %v", r.Form)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"provider-token","token_type":"Bearer"}`)
		case "/userinfo":
			if r.Header.Get("Authorization") != "Bearer provider-token" {
				t.Error("missing provider token")
			}
			_, _ = io.WriteString(w, userinfo)
		default:
			t.Errorf("unexpected request: %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	cf := oauth2x.Config{
		Enable: true, TokenAddr: server.URL + "/token", UserInfoAddr: server.URL + "/userinfo",
		ClientId: "client", ClientSecret: "secret", RedirectURL: server.URL + "/callback", TokenAuthStyle: "params",
		DefaultRoles: []string{"Standard"}, DefaultTeams: []int64{10},
	}
	cf.Attributes.Username, cf.Attributes.Nickname = "login", "name"
	cf.Attributes.Email, cf.Attributes.Phone = "email", "phone"
	rt.Sso.OAuth2 = oauth2x.New(cf)
	require.NoError(t, redis.Set("n9e_oauth_state", "/boards"))
	return rt
}

func requireSSORound7Session(t *testing.T, rt *Router, output CallbackOutput, userID int64) {
	t.Helper()
	token, err := jwt.Parse(output.AccessToken, func(token *jwt.Token) (interface{}, error) {
		require.Equal(t, jwt.SigningMethodHS256, token.Method)
		return []byte(rt.HTTP.JWTAuth.SigningKey), nil
	})
	require.NoError(t, err)
	require.True(t, token.Valid)
	claims := token.Claims.(jwt.MapClaims)
	identity := fmt.Sprintf("%d-alice", userID)
	require.Equal(t, identity, claims["user_identity"])
	saved, err := rt.Redis.Get(context.Background(), rt.wrapJwtKey(claims["access_uuid"].(string))).Result()
	require.NoError(t, err)
	require.Equal(t, identity, saved)
	require.NotEmpty(t, output.RefreshToken)
}

func TestLoginCallbackOAuth_CreatesUserDefaultTeamAndSession(t *testing.T) {
	rt := oauthRound7Router(t, `{"login":"alice","name":"Alice","email":"alice@example.invalid","phone":"12345"}`)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroup{Id: 10, Name: "oncall", UpdateAt: 1}).Error)

	w := identityRequest(t, rt, rt.loginCallbackOAuth, nil, "0?code=valid-code&state=state", nil)

	var output CallbackOutput
	require.Empty(t, identityReply(t, w, &output))
	require.Equal(t, "/boards", output.Redirect)
	require.Equal(t, "alice", output.User.Username)
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved).Error)
	require.Equal(t, "Alice", saved.Nickname)
	require.Equal(t, "alice@example.invalid", saved.Email)
	require.Equal(t, "12345", saved.Phone)
	require.Equal(t, "Standard", saved.Roles)
	require.Equal(t, "oauth2", saved.CreateBy)
	var members []models.UserGroupMember
	require.NoError(t, rt.Ctx.DB.Find(&members).Error)
	require.Equal(t, []models.UserGroupMember{{GroupId: 10, UserId: saved.Id}}, members)
	var group models.UserGroup
	require.NoError(t, rt.Ctx.DB.First(&group, 10).Error)
	require.Greater(t, group.UpdateAt, int64(1))
	requireSSORound7Session(t, rt, output, saved.Id)
	require.Zero(t, rt.Redis.Exists(context.Background(), "n9e_oauth_state").Val())
}

func TestLoginCallbackOAuth_AttributeCoverPreservesLocalRolesAndPassword(t *testing.T) {
	for _, cover := range []bool{false, true} {
		t.Run(fmt.Sprint(cover), func(t *testing.T) {
			rt := oauthRound7Router(t, `{"login":"alice","name":"SSO Alice","email":"new@example.invalid"}`)
			rt.Sso.OAuth2.CoverAttributes = cover
			original := models.User{Id: 7, Username: "alice", Nickname: "Local", Phone: "555", Email: "old@example.invalid",
				Roles: "Admin", Password: "local-hash", Contacts: []byte("{}")}
			require.NoError(t, rt.Ctx.DB.Create(&original).Error)
			require.NoError(t, rt.Redis.Set(context.Background(), "n9e_oauth_state", "/login", 0).Err())

			w := identityRequest(t, rt, rt.loginCallbackOAuth, nil, "0?code=valid-code&state=state", nil)

			var output CallbackOutput
			require.Empty(t, identityReply(t, w, &output))
			require.Equal(t, "/", output.Redirect)
			var saved models.User
			require.NoError(t, rt.Ctx.DB.First(&saved, 7).Error)
			require.Equal(t, "Admin", saved.Roles)
			require.Equal(t, "local-hash", saved.Password)
			require.Equal(t, "555", saved.Phone) // An absent SSO attribute must not erase the local phone.
			if cover {
				require.Equal(t, "SSO Alice", saved.Nickname)
				require.Equal(t, "new@example.invalid", saved.Email)
				require.Equal(t, "oauth2", saved.UpdateBy)
			} else {
				require.Equal(t, original.Nickname, saved.Nickname)
				require.Equal(t, original.Email, saved.Email)
			}
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.UserGroupMember{}).Count(&count).Error)
			require.Zero(t, count) // Default teams apply only when creating an account.
			requireSSORound7Session(t, rt, output, 7)
		})
	}
}

func TestLoginCallbackOAuth_DisabledUserDoesNotGetSession(t *testing.T) {
	rt := oauthRound7Router(t, `{"login":"alice"}`)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Username: "alice", Disabled: 1}).Error)

	w := identityRequest(t, rt, rt.loginCallbackOAuth, nil, "0?code=valid-code&state=state", nil)

	var output CallbackOutput
	require.Contains(t, identityReply(t, w, &output), models.ErrUserDisabled)
	require.Empty(t, output.AccessToken)
	require.Empty(t, output.RefreshToken)
	keys, err := rt.Redis.Keys(context.Background(), "*").Result()
	require.NoError(t, err)
	require.Empty(t, keys)
}

func TestLoginCallbackOAuth_InvalidProviderIdentityDoesNotCreateUserOrSession(t *testing.T) {
	rt := oauthRound7Router(t, `{"error":"expired token"}`)

	w := identityRequest(t, rt, rt.loginCallbackOAuth, nil, "0?code=valid-code&state=state", nil)

	require.Contains(t, identityReply(t, w, nil), "username claim")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.User{}).Count(&count).Error)
	require.Zero(t, count)
	keys, err := rt.Redis.Keys(context.Background(), rt.HTTP.JWTAuth.RedisKeyPrefix+"*").Result()
	require.NoError(t, err)
	require.Empty(t, keys)
}

func TestSSOConfigNameGet_ReturnsOnlyEnabledDisplayNames(t *testing.T) {
	rt, _ := sessionRouter(t)
	ssoRound7Clients(rt)
	rt.Sso.OIDC.Enable, rt.Sso.OIDC.DisplayName = true, "Corporate OIDC"
	rt.Sso.CAS = cas.New(cas.Config{Enable: true, DisplayName: "Corporate CAS"})
	rt.Sso.OAuth2 = oauth2x.New(oauth2x.Config{Enable: true, DisplayName: "Corporate OAuth"})
	rt.Sso.DingTalk = dingtalk.New(dingtalk.Config{Enable: true, DisplayName: "DingTalk"})
	rt.Sso.FeiShu = feishu.New(feishu.Config{Enable: true, DisplayName: "Feishu"})

	w := identityRequest(t, rt, rt.ssoConfigNameGet, nil, "0", nil)

	var output SsoConfigOutput
	require.Empty(t, identityReply(t, w, &output))
	require.Equal(t, SsoConfigOutput{
		OidcDisplayName: "Corporate OIDC", CasDisplayName: "Corporate CAS", OauthDisplayName: "Corporate OAuth",
		DingTalkDisplayName: "DingTalk", FeiShuDisplayName: "Feishu",
	}, output)
	rt.Sso.FeiShu.Enable = false
	w = identityRequest(t, rt, rt.ssoConfigNameGet, nil, "0", nil)
	require.Empty(t, identityReply(t, w, &output))
	require.Empty(t, output.FeiShuDisplayName)
}

func TestLoginCallbackFeishu_UninitializedProviderDoesNotCreateSession(t *testing.T) {
	rt, redis := sessionRouter(t)
	rt.Sso.FeiShu = feishu.New(feishu.Config{})

	w := identityRequest(t, rt, rt.loginCallbackFeiShu, nil, "0?code=invalid&state=state", nil)

	require.Contains(t, identityReply(t, w, nil), "feishu client is not initialized")
	require.Empty(t, redis.Keys())
}

func casRound7Router(t *testing.T, failure bool) *Router {
	t.Helper()
	rt, redis := sessionRouter(t)
	requests := make(chan url.Values, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cas/serviceValidate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		requests <- r.URL.Query()
		if failure {
			_, _ = io.WriteString(w, `<cas:serviceResponse xmlns:cas="http://www.yale.edu/tp/cas">`+
				`<cas:authenticationFailure code="INVALID_TICKET">expired ticket</cas:authenticationFailure></cas:serviceResponse>`)
			return
		}
		_, _ = io.WriteString(w, `<cas:serviceResponse xmlns:cas="http://www.yale.edu/tp/cas">`+
			`<cas:authenticationSuccess><cas:user>alice</cas:user><cas:attributes>`+
			`<cas:displayName>CAS Alice</cas:displayName><cas:mail>alice@example.invalid</cas:mail>`+
			`</cas:attributes></cas:authenticationSuccess></cas:serviceResponse>`)
	}))
	t.Cleanup(server.Close)
	cf := cas.Config{Enable: true, SsoAddr: server.URL + "/cas", RedirectURL: server.URL + "/callback",
		DefaultRoles: []string{"Standard"}}
	cf.Attributes.Nickname, cf.Attributes.Email = "displayName", "mail"
	rt.Sso.CAS = cas.New(cf)
	rt.Sso.CAS.HTTPClient = server.Client()
	require.NoError(t, redis.Set("n9e_cas_state", "/login"))
	t.Cleanup(func() {
		select {
		case query := <-requests:
			require.Equal(t, "ST-valid", query.Get("ticket"))
			require.Equal(t, cf.RedirectURL, query.Get("service"))
		default:
			t.Error("ticket was never validated")
		}
	})
	return rt
}

func TestLoginCallbackCAS_CreatesAccountAndAuthenticatedSession(t *testing.T) {
	rt := casRound7Router(t, false)

	w := identityRequest(t, rt, rt.loginCallbackCas, nil, "0?ticket=ST-valid&state=state", nil)

	var output CallbackOutput
	require.Empty(t, identityReply(t, w, &output))
	require.Equal(t, "/", output.Redirect)
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved).Error)
	require.Equal(t, "alice", saved.Username)
	require.Equal(t, "CAS Alice", saved.Nickname)
	require.Equal(t, "alice@example.invalid", saved.Email)
	require.Equal(t, "cas", saved.Belong)
	require.Equal(t, "Standard", saved.Roles)
	requireSSORound7Session(t, rt, output, saved.Id)
	require.Zero(t, rt.Redis.Exists(context.Background(), "n9e_cas_state").Val())
}

func TestLoginCallbackCAS_CoversAttributesWithoutReplacingCredentials(t *testing.T) {
	rt := casRound7Router(t, false)
	rt.Sso.CAS.CoverAttributes = true
	original := models.User{Id: 7, Username: "alice", Nickname: "Local", Roles: "Viewer", Password: "local-hash"}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)
	require.NoError(t, rt.Redis.Set(context.Background(), "n9e_cas_state", "/boards", 0).Err())

	w := identityRequest(t, rt, rt.loginCallbackCas, nil, "0?ticket=ST-valid&state=state", nil)

	var output CallbackOutput
	require.Empty(t, identityReply(t, w, &output))
	require.Equal(t, "/boards", output.Redirect)
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved, 7).Error)
	require.Equal(t, "CAS Alice", saved.Nickname)
	require.Equal(t, "cas", saved.UpdateBy)
	require.Equal(t, original.Password, saved.Password)
	require.Equal(t, original.Roles, saved.Roles)
	requireSSORound7Session(t, rt, output, 7)
}

func TestLoginCallbackCAS_DisabledUserDoesNotGetSession(t *testing.T) {
	rt := casRound7Router(t, false)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Username: "alice", Disabled: 1}).Error)

	w := identityRequest(t, rt, rt.loginCallbackCas, nil, "0?ticket=ST-valid&state=state", nil)

	require.Contains(t, identityReply(t, w, nil), models.ErrUserDisabled)
	keys, err := rt.Redis.Keys(context.Background(), "*").Result()
	require.NoError(t, err)
	require.Empty(t, keys)
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved).Error)
	require.Equal(t, 1, saved.Disabled)
}

func TestLoginCallbackCAS_InvalidTicketDoesNotCreateAccountOrSession(t *testing.T) {
	rt := casRound7Router(t, true)

	w := identityRequest(t, rt, rt.loginCallbackCas, nil, "0?ticket=ST-valid&state=state", nil)

	require.Contains(t, identityReply(t, w, nil), "INVALID_TICKET")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.User{}).Count(&count).Error)
	require.Zero(t, count)
	keys, err := rt.Redis.Keys(context.Background(), rt.HTTP.JWTAuth.RedisKeyPrefix+"*").Result()
	require.NoError(t, err)
	require.Empty(t, keys)
}

func dingRound7Router(t *testing.T) *Router {
	t.Helper()
	rt, redis := sessionRouter(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1.0/oauth2/userAccessToken":
			_, _ = io.WriteString(w, `{"accessToken":"ding-user-token"}`)
		case "/v1.0/contact/users/me":
			if r.Header.Get("x-acs-dingtalk-access-token") != "ding-user-token" {
				t.Error("missing DingTalk access token")
			}
			_, _ = io.WriteString(w, `{"nick":"alice","mobile":"12345","email":"alice@example.invalid"}`)
		default:
			t.Errorf("unexpected DingTalk request: %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	rt.Sso.DingTalk = dingtalk.New(dingtalk.Config{Enable: true, Endpoint: server.URL, UsernameField: "name",
		DefaultRoles: []string{"Standard"}})
	require.NoError(t, redis.Set("n9e_dingtalk_oauth_state", "/boards"))
	return rt
}

func TestLoginCallbackDingTalk_CreatesProfileAndSession(t *testing.T) {
	rt := dingRound7Router(t)

	w := identityRequest(t, rt, rt.loginCallbackDingTalk, nil, "0?code=valid&state=state", nil)

	var output CallbackOutput
	require.Empty(t, identityReply(t, w, &output))
	require.Equal(t, "/boards", output.Redirect)
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved).Error)
	require.Equal(t, "alice", saved.Username)
	require.Equal(t, "alice@example.invalid", saved.Email)
	require.Equal(t, "12345", saved.Phone)
	require.Equal(t, "dingtalk", saved.Belong)
	require.Equal(t, "Standard", saved.Roles)
	requireSSORound7Session(t, rt, output, saved.Id)
	require.Zero(t, rt.Redis.Exists(context.Background(), "n9e_dingtalk_oauth_state").Val())
}

func TestLoginCallbackDingTalk_CoversProfileAndPreservesLocalPrivileges(t *testing.T) {
	rt := dingRound7Router(t)
	rt.Sso.DingTalk.DingTalkConfig.CoverAttributes = true
	original := models.User{Id: 7, Username: "alice", Nickname: "Local", Roles: "Viewer", Password: "local-hash"}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)
	require.NoError(t, rt.Redis.Set(context.Background(), "n9e_dingtalk_oauth_state", "/login", 0).Err())

	w := identityRequest(t, rt, rt.loginCallbackDingTalk, nil, "0?code=valid&state=state", nil)

	var output CallbackOutput
	require.Empty(t, identityReply(t, w, &output))
	require.Equal(t, "/", output.Redirect)
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved, 7).Error)
	require.Equal(t, "alice", saved.Nickname)
	require.Equal(t, "dingtalk", saved.UpdateBy)
	require.Equal(t, "12345", saved.Phone)
	require.Equal(t, original.Roles, saved.Roles)
	require.Equal(t, original.Password, saved.Password)
	requireSSORound7Session(t, rt, output, 7)
}

func TestLoginCallbackDingTalk_DisabledUserDoesNotGetSession(t *testing.T) {
	rt := dingRound7Router(t)
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Username: "alice", Disabled: 1}).Error)

	w := identityRequest(t, rt, rt.loginCallbackDingTalk, nil, "0?code=valid&state=state", nil)

	require.Contains(t, identityReply(t, w, nil), models.ErrUserDisabled)
	keys, err := rt.Redis.Keys(context.Background(), "*").Result()
	require.NoError(t, err)
	require.Empty(t, keys)
}
