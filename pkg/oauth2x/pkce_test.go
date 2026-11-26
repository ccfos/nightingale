package oauth2x

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

func TestExternalOAuth2PKCE(t *testing.T) {
	var calls int32
	var challenge string
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		verifier := r.FormValue("code_verifier")
		digest := sha256.Sum256([]byte(verifier))
		if (challenge == "" && verifier != "") || (challenge != "" && (verifier == "" || base64.RawURLEncoding.EncodeToString(digest[:]) != challenge)) {
			http.Error(w, "invalid verifier", http.StatusBadRequest)
			return
		}
		if id, secret, ok := r.BasicAuth(); !ok || id != "n9e" || secret != "test-secret" {
			http.Error(w, "invalid client", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"test-token","token_type":"Bearer"}`))
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"staffid":"603758"}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	cf := Config{Enable: true, UsePKCE: true, ClientId: "n9e", ClientSecret: "test-secret", SsoAddr: server.URL + "/authorize",
		TokenAddr: server.URL + "/token", UserInfoAddr: server.URL + "/userinfo", TranTokenMethod: "header", RedirectURL: "https://n9e.example.com/callback/oauth"}
	cf.Attributes.Username = "staffid"
	sso := New(cf)
	_, store := loginRedis(t)
	address, _, err := sso.Authorize(store, "/dashboards", "browser")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(address)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	state := q.Get("state")
	challenge = q.Get("code_challenge")
	if state == "" || len(challenge) != 43 || q.Get("code_challenge_method") != "S256" {
		t.Fatal("authorization request is missing S256 PKCE or state")
	}
	ctx := context.Background()
	if _, err := sso.Callback(store, ctx, "code", "unknown", "browser"); err == nil {
		t.Fatal("callback accepted an unknown state")
	}
	if _, err := sso.Callback(store, ctx, "code", state, "other-browser"); err == nil {
		t.Fatal("callback accepted a different browser")
	}
	out, err := sso.Callback(store, ctx, "code", state, "browser")
	if err != nil {
		t.Fatal(err)
	}
	if out.Redirect != "/dashboards" || out.Username != "603758" {
		t.Fatalf("unexpected callback output: %+v", out)
	}
	if _, err := sso.Callback(store, ctx, "code", state, "browser"); err == nil {
		t.Fatal("callback reused an already consumed state")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected one token request, got %d", got)
	}
	cf.UsePKCE = false
	sso.Reload(cf)
	address, cookieState, err := sso.Authorize(store, "/dashboards", "")
	if err != nil {
		t.Fatal(err)
	}
	u, err = url.Parse(address)
	if err != nil {
		t.Fatal(err)
	}
	if cookieState != "" || u.Query().Has("code_challenge") {
		t.Fatal("legacy authorization sent a PKCE challenge")
	}
	challenge = ""
	out, err = sso.Callback(store, ctx, "legacy-code", u.Query().Get("state"), "")
	if err != nil || out.Redirect != "/dashboards" {
		t.Fatalf("legacy login or local redirect failed: %v, %v", out, err)
	}

}
