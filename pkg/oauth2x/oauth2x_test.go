package oauth2x

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestRSTokenCacheSweep verifies that once the cache crosses the sweep
// threshold, a Put drops already-expired entries so rotated tokens (which are
// never looked up again) cannot accumulate without bound.
func TestRSTokenCacheSweep(t *testing.T) {
	s := &SsoClient{rsTokenCache: make(map[string]rsTokenCacheEntry)}
	past := time.Now().Unix() - 1
	for i := 0; i < rsTokenCacheSweepThreshold; i++ {
		s.rsTokenCache[fmt.Sprintf("expired-%d", i)] = rsTokenCacheEntry{expire: past}
	}

	// This Put crosses the threshold and should sweep all expired entries,
	// leaving only the freshly-inserted live one.
	s.rsTokenCachePut("fresh", &CallbackOutput{Username: "x"}, 60)

	if got := len(s.rsTokenCache); got != 1 {
		t.Errorf("after sweep cache size = %d, want 1 (only the fresh entry)", got)
	}
	if s.rsTokenCacheGet("fresh") == nil {
		t.Error("fresh entry should survive the sweep")
	}
}

func newIntrospectClient(introspectAddr string) *SsoClient {
	s := New(Config{
		Enable:                 true,
		ClientId:               "n9e-rs",
		ClientSecret:           "s3cret",
		RSVerifyMethod:         "introspect",
		IntrospectAddr:         introspectAddr,
		IntrospectCacheSeconds: 0,
	})
	s.Attributes.Username = "username"
	s.Attributes.Nickname = "nickname"
	s.Attributes.Phone = "phone"
	s.Attributes.Email = "email"
	return s
}

func TestVerifyAccessToken(t *testing.T) {
	const audience = "n9e-a2a-rs"

	t.Run("active token with matching audience resolves user", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// RFC 7662: resource server authenticates with its client creds.
			if user, pass, ok := r.BasicAuth(); !ok || user != "n9e-rs" || pass != "s3cret" {
				t.Errorf("missing/wrong basic auth: user=%q ok=%v", user, ok)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			if r.PostForm.Get("token") != "opaque-abc" {
				t.Errorf("token form = %q", r.PostForm.Get("token"))
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"active":true,"aud":"n9e-a2a-rs","username":"alice","nickname":"Alice","email":"a@x.com"}`))
		}))
		defer srv.Close()

		out, err := newIntrospectClient(srv.URL).VerifyAccessToken(context.Background(), "opaque-abc", audience)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Username != "alice" || out.Nickname != "Alice" || out.Email != "a@x.com" {
			t.Errorf("unexpected output: %+v", out)
		}
	})

	t.Run("aud as array containing audience", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"active":true,"aud":["other","n9e-a2a-rs"],"username":"bob"}`))
		}))
		defer srv.Close()
		out, err := newIntrospectClient(srv.URL).VerifyAccessToken(context.Background(), "t", audience)
		if err != nil || out.Username != "bob" {
			t.Fatalf("out=%+v err=%v", out, err)
		}
	})

	t.Run("inactive token rejected", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"active":false}`))
		}))
		defer srv.Close()
		if _, err := newIntrospectClient(srv.URL).VerifyAccessToken(context.Background(), "t", audience); err == nil {
			t.Fatal("expected error for inactive token")
		}
	})

	t.Run("audience mismatch rejected", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"active":true,"aud":"some-other-app","username":"eve"}`))
		}))
		defer srv.Close()
		if _, err := newIntrospectClient(srv.URL).VerifyAccessToken(context.Background(), "t", audience); err == nil {
			t.Fatal("expected error for audience mismatch")
		}
	})

	t.Run("missing audience fails closed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"active":true,"username":"eve"}`))
		}))
		defer srv.Close()
		if _, err := newIntrospectClient(srv.URL).VerifyAccessToken(context.Background(), "t", audience); err == nil {
			t.Fatal("expected error when aud claim is absent")
		}
	})

	t.Run("empty username claim rejected", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"active":true,"aud":"n9e-a2a-rs"}`))
		}))
		defer srv.Close()
		if _, err := newIntrospectClient(srv.URL).VerifyAccessToken(context.Background(), "t", audience); err == nil {
			t.Fatal("expected error for empty username")
		}
	})

	t.Run("empty audience arg rejected before any call", func(t *testing.T) {
		s := newIntrospectClient("http://127.0.0.1:0")
		if _, err := s.VerifyAccessToken(context.Background(), "t", ""); err == nil {
			t.Fatal("expected error for empty audience argument")
		}
	})

	t.Run("missing introspect addr rejected", func(t *testing.T) {
		s := newIntrospectClient("")
		if _, err := s.VerifyAccessToken(context.Background(), "t", audience); err == nil {
			t.Fatal("expected error when IntrospectAddr is empty")
		}
	})

	t.Run("positive result is cached", func(t *testing.T) {
		var hits int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			w.Write([]byte(`{"active":true,"aud":"n9e-a2a-rs","username":"alice","exp":4102444800}`))
		}))
		defer srv.Close()
		s := newIntrospectClient(srv.URL)
		s.IntrospectCacheSeconds = 60

		for i := 0; i < 3; i++ {
			if _, err := s.VerifyAccessToken(context.Background(), "cached-token", audience); err != nil {
				t.Fatalf("call %d: %v", i, err)
			}
		}
		if hits != 1 {
			t.Errorf("introspection endpoint hit %d times, want 1 (cache miss)", hits)
		}
	})
}

func newUserInfoClient(userInfoAddr string) *SsoClient {
	s := New(Config{
		Enable:         true,
		ClientId:       "n9e-rs",
		RSVerifyMethod: "userinfo",
		UserInfoAddr:   userInfoAddr,
	})
	s.Attributes.Username = "username"
	s.Attributes.Nickname = "nickname"
	s.Attributes.Phone = "phone"
	s.Attributes.Email = "email"
	return s
}

func TestVerifyAccessTokenUserInfo(t *testing.T) {
	const audience = "n9e-a2a-rs"

	t.Run("valid token resolved via userinfo (audience not enforced)", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("Authorization"); got != "Bearer good-token" {
				t.Errorf("authorization = %q", got)
			}
			// Note: no aud field — userinfo mode must still succeed.
			w.Write([]byte(`{"username":"carol","nickname":"Carol","email":"c@x.com"}`))
		}))
		defer srv.Close()

		out, err := newUserInfoClient(srv.URL).VerifyAccessToken(context.Background(), "good-token", audience)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Username != "carol" || out.Email != "c@x.com" {
			t.Errorf("unexpected output: %+v", out)
		}
	})

	t.Run("empty RSVerifyMethod defaults to userinfo", func(t *testing.T) {
		var hit bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hit = true
			w.Write([]byte(`{"username":"dave"}`))
		}))
		defer srv.Close()
		s := newUserInfoClient(srv.URL)
		s.RSVerifyMethod = "" // explicit: empty => userinfo

		out, err := s.VerifyAccessToken(context.Background(), "t", audience)
		if err != nil || out.Username != "dave" {
			t.Fatalf("out=%+v err=%v", out, err)
		}
		if !hit {
			t.Error("empty method should have hit the userinfo endpoint")
		}
	})

	t.Run("non-200 from userinfo means invalid token", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer srv.Close()
		if _, err := newUserInfoClient(srv.URL).VerifyAccessToken(context.Background(), "bad", audience); err == nil {
			t.Fatal("expected error when userinfo returns 401")
		}
	})

	t.Run("empty username from userinfo rejected", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"nickname":"no-username"}`))
		}))
		defer srv.Close()
		if _, err := newUserInfoClient(srv.URL).VerifyAccessToken(context.Background(), "t", audience); err == nil {
			t.Fatal("expected error for empty username")
		}
	})

	t.Run("missing userinfo addr rejected", func(t *testing.T) {
		s := newUserInfoClient("")
		if _, err := s.VerifyAccessToken(context.Background(), "t", audience); err == nil {
			t.Fatal("expected error when UserInfoAddr is empty")
		}
	})

	t.Run("userinfo result is cached", func(t *testing.T) {
		var hits int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			w.Write([]byte(`{"username":"carol"}`))
		}))
		defer srv.Close()
		s := newUserInfoClient(srv.URL)
		s.IntrospectCacheSeconds = 60

		for i := 0; i < 3; i++ {
			if _, err := s.VerifyAccessToken(context.Background(), "cached-token", audience); err != nil {
				t.Fatalf("call %d: %v", i, err)
			}
		}
		if hits != 1 {
			t.Errorf("userinfo endpoint hit %d times, want 1 (cache miss)", hits)
		}
	})
}

func TestGetUserinfoField(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		isArray bool
		prefix  string
		field   string
		want    string
	}{
		{
			name:  "no prefix, flat object",
			input: `{"username":"alice","email":"alice@example.com"}`,
			field: "username",
			want:  "alice",
		},
		{
			name:   "single-level prefix (backward compat)",
			input:  `{"data":{"username":"bob","phone":"123"}}`,
			prefix: "data",
			field:  "username",
			want:   "bob",
		},
		{
			name:   "multi-level prefix data.user",
			input:  `{"data":{"user":{"loginName":"charlie","staffPhone":"456"}}}`,
			prefix: "data.user",
			field:  "loginName",
			want:   "charlie",
		},
		{
			name:   "three-level prefix a.b.c",
			input:  `{"a":{"b":{"c":{"name":"deep"}}}}`,
			prefix: "a.b.c",
			field:  "name",
			want:   "deep",
		},
		{
			name:    "no prefix with array",
			input:   `[{"username":"first"},{"username":"second"}]`,
			isArray: true,
			field:   "username",
			want:    "first",
		},
		{
			name:    "single prefix with array",
			input:   `{"data":[{"username":"arrUser"}]}`,
			isArray: true,
			prefix:  "data",
			field:   "username",
			want:    "arrUser",
		},
		{
			name:    "multi-level prefix with array",
			input:   `{"data":{"users":[{"loginName":"arrDeep"}]}}`,
			isArray: true,
			prefix:  "data.users",
			field:   "loginName",
			want:    "arrDeep",
		},
		{
			name:   "literal dot-key prefix (backward compat)",
			input:  `{"data.user":{"username":"dotkey"}}`,
			prefix: "data.user",
			field:  "username",
			want:   "dotkey",
		},
		{
			name:    "literal dot-key prefix with array",
			input:   `{"data.users":[{"username":"arrdot"}]}`,
			isArray: true,
			prefix:  "data.users",
			field:   "username",
			want:    "arrdot",
		},
		{
			name:   "dot-key takes priority over nested path",
			input:  `{"data.user":{"username":"literal"},"data":{"user":{"username":"nested"}}}`,
			prefix: "data.user",
			field:  "username",
			want:   "literal",
		},
		{
			name:   "falls back to nested path when literal key missing",
			input:  `{"data":{"user":{"username":"nested"}}}`,
			prefix: "data.user",
			field:  "username",
			want:   "nested",
		},
		{
			name:   "literal dot-key with empty string value does not fallback",
			input:  `{"data.user":{"username":""},"data":{"user":{"username":"nested"}}}`,
			prefix: "data.user",
			field:  "username",
			want:   "",
		},
		{
			name:    "literal dot-key with empty string value does not fallback (array)",
			input:   `{"data.users":[{"username":""}],"data":{"users":[{"username":"nested"}]}}`,
			isArray: true,
			prefix:  "data.users",
			field:   "username",
			want:    "",
		},
		{
			name:  "missing field returns empty string",
			input: `{"username":"alice"}`,
			field: "nonexistent",
			want:  "",
		},
		{
			name:   "missing prefix path returns empty string",
			input:  `{"data":{"username":"alice"}}`,
			prefix: "no.such.path",
			field:  "username",
			want:   "",
		},
		{
			name:  "empty field returns empty string",
			input: `{"username":"alice"}`,
			field: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getUserinfoField([]byte(tt.input), tt.isArray, tt.prefix, tt.field)
			if got != tt.want {
				t.Errorf("getUserinfoField() = %q, want %q", got, tt.want)
			}
		})
	}
}
