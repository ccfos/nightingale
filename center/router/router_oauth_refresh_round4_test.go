package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
	"github.com/stretchr/testify/require"
)

func TestMCPRefresh_ProducesTokensBoundToOriginalIdentityClientAndResource(t *testing.T) {
	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })
	rt := &Router{}
	rt.HTTP.MCPAuth = httpx.MCPAuth{
		Enable: true, SigningKey: "refresh-test-key", Issuer: "https://n9e.example.invalid",
		Resource: "https://n9e.example.invalid/mcp", AccessTTL: 600, RefreshTTL: 3600,
	}
	refresh, err := rt.mcpSign(jwt.MapClaims{
		"token_use": mcpUseRefresh, "sub": "7", "usr": "alice", "client_id": "client",
		"aud": rt.HTTP.MCPAuth.Resource, "scope": "n9e:read",
	})
	require.NoError(t, err)
	engine := mcpEngine(rt)
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {"client"}}
	req := httptest.NewRequest(http.MethodPost, mcpTokenPath, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var got struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
		ExpiresIn    int    `json:"expires_in"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Equal(t, "Bearer", got.TokenType)
	require.Equal(t, 600, got.ExpiresIn)
	require.Equal(t, "n9e:read", got.Scope)
	for _, tc := range []struct {
		name string
		raw  string
		use  string
	}{
		{name: "access", raw: got.AccessToken, use: mcpUseAccess},
		{name: "refresh", raw: got.RefreshToken, use: mcpUseRefresh},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims, err := rt.mcpParse(tc.raw, tc.use)
			require.NoError(t, err)
			require.Equal(t, "7", claims["sub"])
			require.Equal(t, "alice", claims["usr"])
			require.Equal(t, "client", claims["client_id"])
			require.Equal(t, rt.HTTP.MCPAuth.Resource, claims["aud"])
			require.Equal(t, "n9e:read", claims["scope"])
			require.NotEmpty(t, claims["jti"])
		})
	}
}

func TestMCPRefresh_RejectsInvalidTokenAndMismatchedClient(t *testing.T) {
	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })
	cases := []struct {
		name       string
		use        string
		client     string
		invalidRaw bool
		message    string
	}{
		{name: "malformed", invalidRaw: true, message: "invalid refresh_token"},
		{name: "wrong token use", use: mcpUseAccess, client: "client", message: "invalid refresh_token"},
		{name: "wrong client", use: mcpUseRefresh, client: "other", message: "client_id mismatch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{}
			rt.HTTP.MCPAuth = httpx.MCPAuth{Enable: true, SigningKey: "refresh-test-key"}
			raw := "not-a-token"
			if !tc.invalidRaw {
				var err error
				raw, err = rt.mcpSign(jwt.MapClaims{"token_use": tc.use, "client_id": "client", "sub": "7", "usr": "alice"})
				require.NoError(t, err)
			}
			form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {raw}, "client_id": {tc.client}}
			req := httptest.NewRequest(http.MethodPost, mcpTokenPath, strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()

			mcpEngine(rt).ServeHTTP(w, req)

			require.Equal(t, http.StatusBadRequest, w.Code)
			require.JSONEq(t, `{"error":"invalid_grant","error_description":"`+tc.message+`"}`, w.Body.String())
			require.NotContains(t, w.Body.String(), "access_token")
		})
	}
}

func TestMCPOAuthDecision_DenialPreservesRedirectParametersAndReturnsNoCode(t *testing.T) {
	rt := identityRouter(t, &models.User{})
	rt.HTTP.MCPAuth = httpx.MCPAuth{Enable: true, SigningKey: "decision-test-key"}
	ticket, err := rt.mcpSign(jwt.MapClaims{
		"token_use": mcpUseAuthzReq, "redirect_uri": "https://client.example.invalid/callback?tenant=prod", "state": "state value",
	})
	require.NoError(t, err)

	w := identityRequest(t, rt, rt.MCPOAuthDecision, map[string]string{"req": ticket, "decision": "deny"}, "0", &models.User{Id: 7})

	var got struct {
		Redirect string `json:"redirect"`
	}
	require.Empty(t, identityReply(t, w, &got))
	redirect, err := url.Parse(got.Redirect)
	require.NoError(t, err)
	require.Equal(t, "client.example.invalid", redirect.Host)
	require.Equal(t, "/callback", redirect.Path)
	require.Equal(t, "prod", redirect.Query().Get("tenant"))
	require.Equal(t, "state value", redirect.Query().Get("state"))
	require.Equal(t, "access_denied", redirect.Query().Get("error"))
	require.Empty(t, redirect.Query().Get("code"))
}
