package router

import (
	"encoding/json"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestR8bMCPOAuthCodeValidation(t *testing.T) {
	rt := r8bRouter(t, &models.User{})
	targetReadRedis(t, rt)
	rt.HTTP.MCPAuth.Enable = true
	rt.HTTP.MCPAuth.SigningKey = "r8b-test-key"
	for _, mode := range []string{"invalid", "client", "redirect", "pkce", "resource", "jti"} {
		t.Run(mode, func(t *testing.T) {
			claims := jwt.MapClaims{"token_use": mcpUseCode, "client_id": "client", "redirect_uri": "http://localhost/cb", "code_challenge": pkceChallenge("verifier"), "resource": "resource", "jti": "one"}
			form := url.Values{"grant_type": {"authorization_code"}, "client_id": {"client"}, "redirect_uri": {"http://localhost/cb"}, "code_verifier": {"verifier"}}
			switch mode {
			case "client":
				form.Set("client_id", "other")
			case "redirect":
				form.Set("redirect_uri", "other")
			case "pkce":
				form.Set("code_verifier", "bad")
			case "resource":
				form.Set("resource", "other")
			case "jti":
				delete(claims, "jti")
			}
			raw, err := rt.mcpSign(claims)
			require.NoError(t, err)
			if mode == "invalid" {
				raw = "invalid"
			}
			form.Set("code", raw)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/token", strings.NewReader(form.Encode()))
			c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rt.MCPOAuthToken(c)
			require.Equal(t, 400, w.Code)
			require.Contains(t, w.Body.String(), "invalid_")
			require.False(t, rt.Redis.Exists(c.Request.Context(), mcpCodeRedisPrefix+"one").Val() > 0)
		})
	}
}
func TestR8bMCPOAuthRefreshAndDisabled(t *testing.T) {
	rt := r8bRouter(t, &models.User{})
	targetReadRedis(t, rt)
	rt.HTTP.MCPAuth.Enable = true
	rt.HTTP.MCPAuth.SigningKey = "r8b-test-key"
	rt.HTTP.MCPAuth.AccessTTL = 120
	rt.HTTP.MCPAuth.RefreshTTL = 900
	rt.HTTP.MCPAuth.CodeTTL = 30
	require.Equal(t, 30*time.Second, rt.mcpCodeTTL())
	require.Equal(t, 900*time.Second, rt.mcpRefreshTTL())
	token, err := rt.mcpSign(jwt.MapClaims{"token_use": mcpUseRefresh, "client_id": "client", "sub": "7", "usr": "alice", "aud": "resource", "scope": "read"})
	require.NoError(t, err)
	for _, mode := range []string{"invalid", "client", "success", "unsupported", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			rt.HTTP.MCPAuth.Enable = mode != "disabled"
			form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {token}, "client_id": {"client"}}
			switch mode {
			case "invalid":
				form.Set("refresh_token", "bad")
			case "client":
				form.Set("client_id", "other")
			case "unsupported":
				form.Set("grant_type", "password")
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/token", strings.NewReader(form.Encode()))
			c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rt.MCPOAuthToken(c)
			switch mode {
			case "success":
				require.Equal(t, 200, w.Code)
				var body map[string]interface{}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
				require.Equal(t, float64(120), body["expires_in"])
				claims, err := rt.mcpParse(body["access_token"].(string), mcpUseAccess)
				require.NoError(t, err)
				require.Equal(t, "alice", claims["usr"])
				require.Equal(t, "resource", claims["aud"])
			case "disabled":
				require.Equal(t, 404, w.Code)
			default:
				require.Equal(t, 400, w.Code)
			}
		})
	}
	for _, h := range []gin.HandlerFunc{rt.MCPOAuthServerMetadata, rt.MCPOAuthRegister, rt.MCPOAuthAuthorize} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		h(c)
		require.Equal(t, 404, w.Code)
	}
}
