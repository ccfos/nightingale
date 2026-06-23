package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/center/sso"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/ccfos/nightingale/v6/pkg/oauth2x"
	"github.com/ccfos/nightingale/v6/pkg/oidcx"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
)

// newRSRouter builds a minimal Router wired for RS auth — enough for the
// discovery helpers/handlers, which only read HTTP.RSAuth and Sso.OIDC.
func newRSRouter(enable bool) *Router {
	rt := &Router{}
	rt.HTTP.RSAuth = httpx.RSAuth{Enable: enable, Audience: "n9e-a2a-rs"}
	rt.Sso = &sso.SsoClient{OIDC: &oidcx.SsoClient{Enable: enable, SsoAddr: "https://idp.example.com/realms/x"}}
	return rt
}

// newRSRouterOAuth2 wires a Router for RS auth using the OAuth2 (introspection)
// provider instead of OIDC.
func newRSRouterOAuth2(enable bool) *Router {
	rt := &Router{}
	rt.HTTP.RSAuth = httpx.RSAuth{Enable: enable, Audience: "n9e-a2a-rs", Provider: "oauth2"}
	rt.Sso = &sso.SsoClient{OAuth2: &oauth2x.SsoClient{
		Enable:         enable,
		SsoAddr:        "https://oauth.example.com/authorize",
		RSVerifyMethod: "introspect",
		IntrospectAddr: "https://oauth.example.com/introspect",
	}}
	return rt
}

func TestRSAuthProvider(t *testing.T) {
	if got := newRSRouter(true).rsAuthProvider(); got != "oidc" {
		t.Errorf("default provider = %q, want oidc", got)
	}
	if got := newRSRouterOAuth2(true).rsAuthProvider(); got != "oauth2" {
		t.Errorf("provider = %q, want oauth2", got)
	}
}

func TestRSAuthEnabledOAuth2(t *testing.T) {
	// Helper default is introspect mode with an IntrospectAddr => enabled.
	if newRSRouterOAuth2(true).rsAuthEnabled() != true {
		t.Error("oauth2 introspect RS with addr+enable should be enabled")
	}
	if newRSRouterOAuth2(false).rsAuthEnabled() != false {
		t.Error("disabled oauth2 RS should not be enabled")
	}

	// introspect mode gates on IntrospectAddr.
	rt := newRSRouterOAuth2(true)
	rt.Sso.OAuth2.IntrospectAddr = ""
	if rt.rsAuthEnabled() != false {
		t.Error("introspect mode without IntrospectAddr should not be enabled")
	}

	// Default (empty RSVerifyMethod) is userinfo: gates on UserInfoAddr, and an
	// IntrospectAddr alone is NOT enough.
	def := newRSRouterOAuth2(true)
	def.Sso.OAuth2.RSVerifyMethod = ""
	if def.rsAuthEnabled() != false {
		t.Error("default(userinfo) mode with only IntrospectAddr should not be enabled")
	}
	def.Sso.OAuth2.UserInfoAddr = "https://oauth.example.com/userinfo"
	if def.rsAuthEnabled() != true {
		t.Error("default(userinfo) mode with UserInfoAddr should be enabled")
	}
}

func TestShouldVerifyAsRS(t *testing.T) {
	jwtWithIss, err := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.MapClaims{"iss": "https://idp.example.com", "sub": "alice"}).SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	jwtNoIss, err := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.MapClaims{"sub": "alice"}).SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	opaque := "0c2c1f0e-1111-2222-3333-444455556666"

	// OIDC provider: only JWTs carrying an issuer go through RS.
	oidcRT := newRSRouter(true)
	if !oidcRT.shouldVerifyAsRS(jwtWithIss) {
		t.Error("oidc: jwt with iss should verify as RS")
	}
	if oidcRT.shouldVerifyAsRS(jwtNoIss) {
		t.Error("oidc: session jwt (no iss) must not verify as RS")
	}
	if oidcRT.shouldVerifyAsRS(opaque) {
		t.Error("oidc: opaque token must not verify as RS")
	}

	// OAuth2 provider: opaque tokens go through RS, JWTs (n9e session) do not.
	oauthRT := newRSRouterOAuth2(true)
	if !oauthRT.shouldVerifyAsRS(opaque) {
		t.Error("oauth2: opaque token should verify as RS")
	}
	if oauthRT.shouldVerifyAsRS(jwtNoIss) {
		t.Error("oauth2: session jwt must not be sent to introspection")
	}
}

func TestOIDCDiscoveryURLOAuth2Provider(t *testing.T) {
	// A plain OAuth2 IdP has no OIDC discovery doc — the AgentCard must not
	// advertise an OIDC scheme for it.
	if got := newRSRouterOAuth2(true).oidcDiscoveryURL(); got != "" {
		t.Errorf("oauth2 provider should yield empty OIDC discovery URL, got %q", got)
	}
}

func TestOAuthProtectedResourceOAuth2Provider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)

	newRSRouterOAuth2(true).oauthProtectedResource(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body struct {
		AuthorizationServers []string `json:"authorization_servers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(body.AuthorizationServers) != 1 || body.AuthorizationServers[0] != "https://oauth.example.com/authorize" {
		t.Errorf("authorization_servers = %v, want oauth2 SsoAddr", body.AuthorizationServers)
	}
}

func TestOIDCDiscoveryURL(t *testing.T) {
	if got := newRSRouter(false).oidcDiscoveryURL(); got != "" {
		t.Errorf("RS disabled should yield empty discovery URL, got %q", got)
	}
	want := "https://idp.example.com/realms/x/.well-known/openid-configuration"
	if got := newRSRouter(true).oidcDiscoveryURL(); got != want {
		t.Errorf("discovery URL = %q, want %q", got, want)
	}
}

func TestOAuthProtectedResource(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("enabled serves RFC 9728 metadata", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)

		newRSRouter(true).oauthProtectedResource(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var body struct {
			Resource               string   `json:"resource"`
			AuthorizationServers   []string `json:"authorization_servers"`
			BearerMethodsSupported []string `json:"bearer_methods_supported"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if body.Resource != "n9e-a2a-rs" {
			t.Errorf("resource = %q, want n9e-a2a-rs", body.Resource)
		}
		if len(body.AuthorizationServers) != 1 || body.AuthorizationServers[0] != "https://idp.example.com/realms/x" {
			t.Errorf("authorization_servers = %v", body.AuthorizationServers)
		}
		if len(body.BearerMethodsSupported) != 1 || body.BearerMethodsSupported[0] != "header" {
			t.Errorf("bearer_methods_supported = %v", body.BearerMethodsSupported)
		}
	})

	t.Run("disabled returns 404", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)

		newRSRouter(false).oauthProtectedResource(c)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})
}

func TestTokenHasIssuer(t *testing.T) {
	sign := func(claims jwt.MapClaims) string {
		signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("secret"))
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		return signed
	}

	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{
			// Shape minted by createTokens — no iss claim, must stay on the
			// existing self-signed session-JWT path.
			name: "session jwt has no issuer",
			raw:  sign(jwt.MapClaims{"authorized": true, "access_uuid": "u", "user_identity": "1-alice"}),
			want: false,
		},
		{
			name: "idp access token has issuer",
			raw:  sign(jwt.MapClaims{"iss": "https://idp.example.com", "sub": "alice"}),
			want: true,
		},
		{
			name: "empty issuer is not an idp token",
			raw:  sign(jwt.MapClaims{"iss": "", "sub": "alice"}),
			want: false,
		},
		{name: "garbage is not a jwt", raw: "not-a-jwt", want: false},
		{name: "empty string", raw: "", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tokenHasIssuer(tc.raw); got != tc.want {
				t.Errorf("tokenHasIssuer() = %v, want %v", got, tc.want)
			}
		})
	}
}
