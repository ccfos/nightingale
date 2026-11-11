package oauth2x

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/oauth2"
)

// fake4A mimics a "4A"-style IdP: credentials only in the form body, token and
// userinfo responses wrapped as {"code":200,"msg":"ok","data":{...}}, and
// business errors served with HTTP 200.
type fake4A struct {
	t             *testing.T
	tokenCalls    int32
	tokenBody     string // response for /token; empty means a successful wrapped token
	userinfoCode  int
	userinfoBody  string // response for /userinfo; empty means a successful wrapped user
	sawBasicAuth  int32
	lastTokenForm map[string]string
}

func (f *fake4A) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/uap/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&f.tokenCalls, 1)
		if _, _, ok := r.BasicAuth(); ok {
			atomic.AddInt32(&f.sawBasicAuth, 1)
		}
		if err := r.ParseForm(); err != nil {
			f.t.Fatalf("parse token form: %v", err)
		}
		f.lastTokenForm = map[string]string{}
		for k := range r.PostForm {
			f.lastTokenForm[k] = r.PostForm.Get(k)
		}
		w.Header().Set("Content-Type", "text/plain;charset=UTF-8")
		if f.tokenBody != "" {
			w.Write([]byte(f.tokenBody))
			return
		}
		if r.PostForm.Get("client_secret") != "s3cret" {
			w.Write([]byte(`{"code":30119,"msg":"无效client_secret"}`))
			return
		}
		w.Write([]byte(`{"code":200,"msg":"ok","data":{"access_token":"at-1","refresh_token":"rt-1","expires_in":86399,"client_id":"n9e","scope":"userinfo","openid":"3381","subAccount":{"name":"testuser"}}}`))
	})
	mux.HandleFunc("/uap/oauth2/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			f.t.Fatalf("parse userinfo form: %v", err)
		}
		if f.userinfoCode != 0 {
			w.WriteHeader(f.userinfoCode)
		}
		if f.userinfoBody != "" {
			w.Write([]byte(f.userinfoBody))
			return
		}
		if r.PostForm.Get("access_token") != "at-1" {
			w.Write([]byte(`{"code":"30106","msg":"无效access_token"}`))
			return
		}
		w.Write([]byte(`{"code":"200","msg":"ok","data":{"openid":"3381","username":"user1","phone":"18909897021","fullName":"张三"}}`))
	})
	return mux
}

func new4AClient(addr string) *SsoClient {
	cf := Config{
		Enable:              true,
		RedirectURL:         "http://n9e.local/callback/oauth",
		SsoAddr:             addr + "/uap/oauth2/authorize",
		TokenAddr:           addr + "/uap/oauth2/token",
		TokenAuthStyle:      "params",
		TokenResponsePrefix: "data",
		UserInfoAddr:        addr + "/uap/oauth2/userinfo",
		TranTokenMethod:     "formdata",
		ClientId:            "n9e",
		ClientSecret:        "s3cret",
		UserinfoPrefix:      "data",
		Scopes:              []string{"userinfo"},
	}
	cf.Attributes.Username = "username"
	cf.Attributes.Nickname = "fullName"
	cf.Attributes.Phone = "phone"
	return New(cf)
}

func TestExchangeUserNonStandardIdP(t *testing.T) {
	t.Run("wrapped token and userinfo resolve the user", func(t *testing.T) {
		f := &fake4A{t: t}
		srv := httptest.NewServer(f.handler())
		defer srv.Close()

		out, err := new4AClient(srv.URL).exchangeUser("code-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Username != "user1" || out.Nickname != "张三" || out.Phone != "18909897021" || out.AccessToken != "at-1" {
			t.Errorf("unexpected output: %+v", out)
		}
		if f.tokenCalls != 1 {
			t.Errorf("token endpoint called %d times, want 1 (code must not be replayed)", f.tokenCalls)
		}
		if f.sawBasicAuth != 0 {
			t.Error("TokenAuthStyle=params must not send HTTP Basic credentials")
		}
		for k, want := range map[string]string{
			"grant_type":    "authorization_code",
			"code":          "code-1",
			"client_id":     "n9e",
			"client_secret": "s3cret",
			"redirect_uri":  "http://n9e.local/callback/oauth",
		} {
			if got := f.lastTokenForm[k]; got != want {
				t.Errorf("token form %s = %q, want %q", k, got, want)
			}
		}
	})

	t.Run("business error with http 200 surfaces the IdP message", func(t *testing.T) {
		f := &fake4A{t: t, tokenBody: `{"code":30110,"msg":"无效code"}`}
		srv := httptest.NewServer(f.handler())
		defer srv.Close()

		_, err := new4AClient(srv.URL).exchangeUser("bad-code")
		if err == nil {
			t.Fatal("expected error for business error response")
		}
		if !strings.Contains(err.Error(), "30110") || !strings.Contains(err.Error(), "无效code") {
			t.Errorf("error should carry the IdP response, got: %v", err)
		}
		if f.tokenCalls != 1 {
			t.Errorf("token endpoint called %d times, want 1", f.tokenCalls)
		}
	})

	t.Run("userinfo business error must not yield a blank username", func(t *testing.T) {
		f := &fake4A{t: t, userinfoBody: `{"code":"30106","msg":"无效access_token"}`}
		srv := httptest.NewServer(f.handler())
		defer srv.Close()

		out, err := new4AClient(srv.URL).exchangeUser("code-1")
		if err == nil {
			t.Fatalf("expected error, got output %+v", out)
		}
	})

	t.Run("userinfo non-2xx is rejected", func(t *testing.T) {
		f := &fake4A{t: t, userinfoCode: http.StatusUnauthorized}
		srv := httptest.NewServer(f.handler())
		defer srv.Close()

		if _, err := new4AClient(srv.URL).exchangeUser("code-1"); err == nil || !strings.Contains(err.Error(), "401") {
			t.Fatalf("expected http 401 error, got %v", err)
		}
	})
}

// TestExchangeUserStandardIdP guards the default path: without the new
// options a standard IdP keeps working via auto-detected Basic auth.
func TestExchangeUserStandardIdP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "n9e" || pass != "s3cret" {
			t.Errorf("expected Basic auth, got user=%q ok=%v", user, ok)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"at-std","token_type":"Bearer","expires_in":3600}`))
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer at-std" {
			t.Errorf("unexpected Authorization header %q", r.Header.Get("Authorization"))
		}
		w.Write([]byte(`{"sub":"alice","nickname":"Alice"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cf := Config{
		Enable:       true,
		TokenAddr:    srv.URL + "/token",
		UserInfoAddr: srv.URL + "/userinfo",
		ClientId:     "n9e",
		ClientSecret: "s3cret",
	}
	cf.Attributes.Username = "sub"
	cf.Attributes.Nickname = "nickname"

	out, err := New(cf).exchangeUser("code-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Username != "alice" || out.Nickname != "Alice" || out.AccessToken != "at-std" {
		t.Errorf("unexpected output: %+v", out)
	}
}

func TestUnwrapTokenResponse(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		prefix  string
		want    string
		wantErr bool
	}{
		{
			name:   "single-level prefix",
			body:   `{"code":200,"data":{"access_token":"a","expires_in":10}}`,
			prefix: "data",
			want:   `{"access_token":"a","expires_in":10}`,
		},
		{
			name:   "multi-level prefix",
			body:   `{"result":{"token":{"access_token":"b"}}}`,
			prefix: "result.token",
			want:   `{"access_token":"b"}`,
		},
		{
			name:   "already standard passes through",
			body:   `{"access_token":"c","token_type":"Bearer"}`,
			prefix: "data",
			want:   `{"access_token":"c","token_type":"Bearer"}`,
		},
		{name: "business error", body: `{"code":30110,"msg":"bad code"}`, prefix: "data", wantErr: true},
		{name: "null data", body: `{"code":500,"data":null}`, prefix: "data", wantErr: true},
		{name: "non-object data", body: `{"code":500,"data":"oops"}`, prefix: "data", wantErr: true},
		{name: "empty data object", body: `{"code":500,"msg":"x","data":{}}`, prefix: "data", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(unwrapTokenResponse([]byte(tt.body), tt.prefix))
			if tt.wantErr {
				if !strings.Contains(got, `"error":"invalid_token_response"`) || !strings.Contains(got, "error_description") {
					t.Errorf("expected RFC 6749 error body, got %s", got)
				}
				return
			}
			if got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestParseAuthStyle(t *testing.T) {
	for in, want := range map[string]oauth2.AuthStyle{
		"params":   oauth2.AuthStyleInParams,
		" Params ": oauth2.AuthStyleInParams,
		"header":   oauth2.AuthStyleInHeader,
		"":         oauth2.AuthStyleAutoDetect,
		"bogus":    oauth2.AuthStyleAutoDetect,
	} {
		if got := parseAuthStyle(in); got != want {
			t.Errorf("parseAuthStyle(%q) = %d, want %d", in, got, want)
		}
	}
}
