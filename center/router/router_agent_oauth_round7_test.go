package router

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/ccfos/nightingale/v6/pkg/oauth2x"
	"github.com/golang-jwt/jwt"
	"github.com/stretchr/testify/require"
)

func agentOAuthRound7Router(t *testing.T, active bool) *Router {
	t.Helper()
	rt, _ := sessionRouter(t)
	require.NoError(t, rt.Ctx.DB.AutoMigrate(&models.UserGroup{}, &models.UserGroupMember{}))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("token") != "opaque-token" {
			t.Errorf("unexpected token: %v", r.Form)
		}
		if r.Form.Get("token_type_hint") != "access_token" {
			t.Error("missing token type hint")
		}
		username, password, ok := r.BasicAuth()
		if !ok || username != "agent-client" || password != "fake-secret" {
			t.Error("wrong introspection credentials")
		}
		if !active {
			_, _ = io.WriteString(w, `{"active":false}`)
			return
		}
		_, _ = io.WriteString(w, `{"active":true,"aud":"n9e-agent","login":"alice","name":"Agent Alice",`+
			`"email":"alice@example.invalid","phone":"12345"}`)
	}))
	t.Cleanup(server.Close)
	cf := oauth2x.Config{Enable: true, RSVerifyMethod: "introspect", IntrospectAddr: server.URL,
		ClientId: "agent-client", ClientSecret: "fake-secret", DefaultRoles: []string{"Standard"}, DefaultTeams: []int64{10}}
	cf.Attributes.Username, cf.Attributes.Nickname = "login", "name"
	cf.Attributes.Email, cf.Attributes.Phone = "email", "phone"
	rt.Sso.OAuth2 = oauth2x.New(cf)
	rt.HTTP.RSAuth = httpx.RSAuth{Enable: true, Provider: "oauth2", Audience: "n9e-agent"}
	return rt
}

func TestVerifyAgentOAuthToken_ProvisionsIdentityAndDefaultTeam(t *testing.T) {
	rt := agentOAuthRound7Router(t, true)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroup{Id: 10, Name: "oncall", UpdateAt: 1}).Error)

	id, username, ok := rt.VerifyAgentOAuthToken(context.Background(), "opaque-token")

	require.True(t, ok)
	require.Equal(t, "alice", username)
	require.Positive(t, id)
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved, id).Error)
	require.Equal(t, "Agent Alice", saved.Nickname)
	require.Equal(t, "alice@example.invalid", saved.Email)
	require.Equal(t, "12345", saved.Phone)
	require.Equal(t, "oauth2", saved.Belong)
	require.Equal(t, "Standard", saved.Roles)
	var members []models.UserGroupMember
	require.NoError(t, rt.Ctx.DB.Find(&members).Error)
	require.Equal(t, []models.UserGroupMember{{GroupId: 10, UserId: id}}, members)
	var group models.UserGroup
	require.NoError(t, rt.Ctx.DB.First(&group, 10).Error)
	require.Greater(t, group.UpdateAt, int64(1))
}

func TestVerifyAgentOAuthToken_ExistingIdentityKeepsLocalProfileAndMemberships(t *testing.T) {
	rt := agentOAuthRound7Router(t, true)
	original := models.User{Id: 7, Username: "alice", Nickname: "Local Alice", Roles: "Viewer", Password: "local-hash"}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)

	id, username, ok := rt.VerifyAgentOAuthToken(context.Background(), "opaque-token")

	require.True(t, ok)
	require.Equal(t, int64(7), id)
	require.Equal(t, "alice", username)
	var saved models.User
	require.NoError(t, rt.Ctx.DB.First(&saved, 7).Error)
	require.Equal(t, original.Nickname, saved.Nickname)
	require.Equal(t, original.Password, saved.Password)
	require.Equal(t, original.Roles, saved.Roles)
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.UserGroupMember{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestVerifyAgentOAuthToken_InactiveTokenDoesNotProvisionUser(t *testing.T) {
	rt := agentOAuthRound7Router(t, false)

	id, username, ok := rt.VerifyAgentOAuthToken(context.Background(), "opaque-token")

	require.False(t, ok)
	require.Zero(t, id)
	require.Empty(t, username)
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.User{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestVerifyAgentOAuthToken_BuiltinAccessTokenResolvesSignedIdentity(t *testing.T) {
	rt := identityRouter(t, &models.User{})
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 7, Username: "alice"}).Error)
	rt.HTTP.MCPAuth = httpx.MCPAuth{
		Enable: true, SigningKey: "round7-agent-key", Issuer: "http://127.0.0.1", Resource: "http://127.0.0.1/mcp",
	}
	token, err := rt.mcpSign(jwt.MapClaims{
		"token_use": mcpUseAccess, "sub": "7", "usr": "alice", "aud": rt.HTTP.MCPAuth.Resource,
	})
	require.NoError(t, err)

	id, username, ok := rt.VerifyAgentOAuthToken(context.Background(), token)

	require.True(t, ok)
	require.Equal(t, int64(7), id)
	require.Equal(t, "alice", username)
}

func TestVerifyAgentOAuthToken_RejectsEmptyAndTamperedBuiltinTokens(t *testing.T) {
	rt := &Router{}
	rt.HTTP.MCPAuth = httpx.MCPAuth{Enable: true, SigningKey: "round7-agent-key", Resource: "http://127.0.0.1/mcp"}
	for _, raw := range []string{"", "not.a.token"} {
		id, username, ok := rt.VerifyAgentOAuthToken(context.Background(), raw)

		require.False(t, ok)
		require.Zero(t, id)
		require.Empty(t, username)
	}
}
