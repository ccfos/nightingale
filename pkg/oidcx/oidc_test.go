package oidcx

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	oidc "github.com/coreos/go-oidc"
	"github.com/golang-jwt/jwt"
)

// newTestIdP spins up a minimal OIDC IdP: an OpenID discovery document plus a
// JWKS exposing one RSA public key, and returns an oidc.Provider wired to it
// together with the private key used to mint access tokens and the issuer URL.
func newTestIdP(t *testing.T) (*oidc.Provider, *rsa.PrivateKey, string) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	pub := priv.PublicKey
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"issuer":%q,"authorization_endpoint":%q,"token_endpoint":%q,"jwks_uri":%q}`,
			srv.URL, srv.URL+"/auth", srv.URL+"/token", srv.URL+"/jwks")
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"keys":[{"kty":"RSA","alg":"RS256","use":"sig","n":%q,"e":%q}]}`, n, e)
	})

	provider, err := oidc.NewProvider(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	return provider, priv, srv.URL
}

func signRS256(t *testing.T, priv *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(priv)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func newTestClient(provider *oidc.Provider) *SsoClient {
	s := &SsoClient{Enable: true, Provider: provider}
	s.Attributes.Username = "sub"
	s.Attributes.Nickname = "nickname"
	s.Attributes.Phone = "phone_number"
	s.Attributes.Email = "email"
	return s
}

func TestVerifyAccessToken(t *testing.T) {
	provider, priv, issuer := newTestIdP(t)
	s := newTestClient(provider)
	const audience = "n9e-agent"
	ctx := context.Background()

	t.Run("valid token maps claims to user", func(t *testing.T) {
		token := signRS256(t, priv, jwt.MapClaims{
			"iss":      issuer,
			"aud":      audience,
			"sub":      "alice",
			"nickname": "Alice",
			"email":    "alice@example.com",
			"exp":      time.Now().Add(time.Hour).Unix(),
		})
		out, err := s.VerifyAccessToken(ctx, token, audience)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Username != "alice" {
			t.Errorf("username = %q, want alice", out.Username)
		}
		if out.Nickname != "Alice" || out.Email != "alice@example.com" {
			t.Errorf("claims not mapped: %+v", out)
		}
	})

	t.Run("audience not bound to this service is rejected", func(t *testing.T) {
		token := signRS256(t, priv, jwt.MapClaims{
			"iss": issuer,
			"aud": "some-other-app",
			"sub": "alice",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		if _, err := s.VerifyAccessToken(ctx, token, audience); err == nil {
			t.Fatal("expected error when aud does not contain this service")
		}
	})

	t.Run("expired token is rejected", func(t *testing.T) {
		token := signRS256(t, priv, jwt.MapClaims{
			"iss": issuer,
			"aud": audience,
			"sub": "alice",
			"exp": time.Now().Add(-time.Hour).Unix(),
		})
		if _, err := s.VerifyAccessToken(ctx, token, audience); err == nil {
			t.Fatal("expected error for expired token")
		}
	})

	t.Run("wrong issuer is rejected", func(t *testing.T) {
		token := signRS256(t, priv, jwt.MapClaims{
			"iss": "https://evil.example.com",
			"aud": audience,
			"sub": "alice",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		if _, err := s.VerifyAccessToken(ctx, token, audience); err == nil {
			t.Fatal("expected error for wrong issuer")
		}
	})

	t.Run("bad signature is rejected", func(t *testing.T) {
		other, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generate rsa key: %v", err)
		}
		token := signRS256(t, other, jwt.MapClaims{
			"iss": issuer,
			"aud": audience,
			"sub": "alice",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		if _, err := s.VerifyAccessToken(ctx, token, audience); err == nil {
			t.Fatal("expected error for token signed by an unknown key")
		}
	})

	t.Run("empty username claim is rejected", func(t *testing.T) {
		token := signRS256(t, priv, jwt.MapClaims{
			"iss": issuer,
			"aud": audience,
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		if _, err := s.VerifyAccessToken(ctx, token, audience); err == nil {
			t.Fatal("expected error when the username claim is empty")
		}
	})

	t.Run("disabled oidc is rejected", func(t *testing.T) {
		if _, err := (&SsoClient{Enable: false}).VerifyAccessToken(ctx, "x", audience); err == nil {
			t.Fatal("expected error when oidc is disabled")
		}
	})

	t.Run("unconfigured audience is rejected", func(t *testing.T) {
		if _, err := s.VerifyAccessToken(ctx, "x", ""); err == nil {
			t.Fatal("expected error when audience is not configured")
		}
	})
}
