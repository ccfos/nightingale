package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/center/sso"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
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
