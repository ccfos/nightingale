package a2a

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

func TestBuildAgentCardSecuritySchemes(t *testing.T) {
	t.Run("without oidc only advertises x-user-token", func(t *testing.T) {
		card := buildAgentCard("https://n9e.example.com/a2a", "X-User-Token", "")

		if _, ok := card.SecuritySchemes[securitySchemeName]; !ok {
			t.Error("x-user-token scheme must be present")
		}
		if _, ok := card.SecuritySchemes[oidcSecuritySchemeName]; ok {
			t.Error("oidc scheme must be absent when discovery URL is empty")
		}
		if len(card.SecurityRequirements) != 1 {
			t.Errorf("want 1 security requirement, got %d", len(card.SecurityRequirements))
		}
	})

	t.Run("with oidc advertises both schemes as satisfy-any", func(t *testing.T) {
		const disc = "https://idp.example.com/realms/x/.well-known/openid-configuration"
		card := buildAgentCard("https://n9e.example.com/a2a", "X-User-Token", disc)

		if _, ok := card.SecuritySchemes[securitySchemeName]; !ok {
			t.Error("x-user-token scheme must still be present")
		}
		scheme, ok := card.SecuritySchemes[oidcSecuritySchemeName]
		if !ok {
			t.Fatal("oidc scheme must be present when discovery URL is set")
		}
		oidc, ok := scheme.(a2a.OpenIDConnectSecurityScheme)
		if !ok {
			t.Fatalf("oidc scheme has wrong type %T", scheme)
		}
		if oidc.OpenIDConnectURL != disc {
			t.Errorf("OpenIDConnectURL = %q, want %q", oidc.OpenIDConnectURL, disc)
		}
		// satisfy-any is expressed as two separate requirement alternatives.
		if len(card.SecurityRequirements) != 2 {
			t.Errorf("want 2 security requirements (satisfy-any), got %d", len(card.SecurityRequirements))
		}
	})
}

// TestAgentCardHandlerResolvesOIDCPerRequest guards the fix for the startup
// snapshot bug: the oidc scheme must be resolved per request, so enabling
// RS/OIDC at runtime is reflected in the card without rebuilding the handler
// (i.e. without a center restart).
func TestAgentCardHandlerResolvesOIDCPerRequest(t *testing.T) {
	enabled := false
	h := AgentCardHandler(AgentCardOptions{
		BaseURL:         "https://n9e.example.com",
		A2APath:         "/a2a",
		TokenHeaderName: "X-User-Token",
		OIDCDiscoveryURL: func() string {
			if enabled {
				return "https://idp.example.com/realms/x/.well-known/openid-configuration"
			}
			return ""
		},
	})

	advertisesOIDC := func() bool {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/.well-known/agent-card.json", nil)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var card struct {
			SecuritySchemes map[string]json.RawMessage `json:"securitySchemes"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &card); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		_, ok := card.SecuritySchemes[string(oidcSecuritySchemeName)]
		return ok
	}

	if advertisesOIDC() {
		t.Error("oidc scheme must be absent while the resolver returns empty")
	}
	enabled = true // simulate OIDC/RS becoming effective at runtime
	if !advertisesOIDC() {
		t.Error("oidc scheme must appear on the next request once enabled — no restart needed")
	}
}
