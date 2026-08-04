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

// deadIdPAddr returns the address of a server that has already been shut down,
// so discovery fails fast with connection refused instead of hanging on DNS.
func deadIdPAddr(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.NewServeMux())
	addr := srv.URL
	srv.Close()
	return addr
}

func newReloadConfig(ssoAddr string) Config {
	return Config{
		Enable:      true,
		DisplayName: "Sign in with OIDC",
		SsoAddr:     ssoAddr,
		RedirectURL: "http://n9e.com/callback",
		ClientId:    "n9e",
	}
}

func TestReloadRetriesUntilIdPIsReachable(t *testing.T) {
	_, _, issuer := newTestIdP(t)

	cf := newReloadConfig(deadIdPAddr(t))
	s, err := New(cf)
	if err == nil {
		t.Fatal("expected an error when the idp is unreachable")
	}
	if s == nil {
		t.Fatal("New must return a usable client even when the idp is unreachable")
	}
	if s.Ready() {
		t.Error("client must not be ready before discovery succeeds")
	}
	if got := s.GetDisplayName(); got != "" {
		t.Errorf("display name = %q, want empty so the login page hides the entry", got)
	}

	// IdP 恢复后在同一个客户端上重试即可补齐，不需要重启进程
	cf.SsoAddr = issuer
	if err := s.Reload(cf); err != nil {
		t.Fatalf("reload after the idp came back: %v", err)
	}
	if !s.Ready() {
		t.Fatal("client should be ready once discovery succeeds")
	}
	if got := s.GetDisplayName(); got != cf.DisplayName {
		t.Errorf("display name = %q, want %q", got, cf.DisplayName)
	}
	if s.Attributes.Username != "sub" || len(s.Config.Scopes) == 0 {
		t.Errorf("defaults not applied: username=%q scopes=%v", s.Attributes.Username, s.Config.Scopes)
	}
	if _, ok := s.Ctx.Deadline(); ok {
		t.Error("stored ctx must not carry the discovery timeout, later token exchanges would expire")
	}
}

func TestReloadFailureHidesLoginEntry(t *testing.T) {
	_, _, issuer := newTestIdP(t)

	cf := newReloadConfig(issuer)
	cf.SsoLogoutAddr = issuer + "/session/end"
	s, err := New(cf)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if !s.Ready() {
		t.Fatal("client should be ready when the idp is reachable")
	}

	// 一个曾经可用的客户端在 IdP 不可达后要转为未就绪，而不是留下半初始化的入口
	cf.SsoAddr = deadIdPAddr(t)
	if err := s.Reload(cf); err == nil {
		t.Fatal("expected an error when the idp becomes unreachable")
	}
	if s.Ready() {
		t.Error("client must not stay ready after discovery failed")
	}
	if got := s.GetDisplayName(); got != "" {
		t.Errorf("display name = %q, want empty", got)
	}
	if got := s.GetSsoLogoutAddr("id-token"); got != "" {
		t.Errorf("logout addr = %q, want empty", got)
	}
}

// 初始化失败时 center 曾把 OIDC 客户端置空，定期 reload 再解引用它导致进程崩溃
func TestNilClientIsSafe(t *testing.T) {
	var s *SsoClient
	if s.Ready() {
		t.Error("nil client must not report ready")
	}
	if got := s.GetDisplayName(); got != "" {
		t.Errorf("display name = %q, want empty", got)
	}
	if got := s.GetSsoLogoutAddr("id-token"); got != "" {
		t.Errorf("logout addr = %q, want empty", got)
	}
}

// Provider 里的远程 JWKS 一直用 Reload 时传进去的 ctx 取密钥，所以那个 ctx 不能是
// 用完就 cancel 的短生命周期上下文，否则客户端 Ready 为真、验签却全部 context canceled
func TestClientBuiltByReloadCanVerifyTokens(t *testing.T) {
	_, priv, issuer := newTestIdP(t)

	s, err := New(newReloadConfig(issuer))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	const audience = "n9e-agent"
	token := signRS256(t, priv, jwt.MapClaims{
		"iss": issuer,
		"aud": audience,
		"sub": "alice",
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	if _, err := s.VerifyAccessToken(context.Background(), token, audience); err != nil {
		t.Fatalf("verify token with a client built by Reload: %v", err)
	}
}

// 定期 reload 和管理员保存配置会并发调用 Reload，慢的旧配置不能盖掉先完成的新配置
func TestSlowReloadDoesNotOverwriteNewerConfig(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"issuer":%q,"authorization_endpoint":%q,"token_endpoint":%q,"jwks_uri":%q}`,
			srv.URL, srv.URL+"/auth", srv.URL+"/token", srv.URL+"/jwks")
	})

	s := &SsoClient{}
	slowDone := make(chan error, 1)
	go func() { slowDone <- s.Reload(newReloadConfig(srv.URL)) }()
	<-entered // 旧配置已经卡在 discovery 上

	// 管理员此刻把 OIDC 关掉
	disableDone := make(chan error, 1)
	disableStarted := make(chan struct{})
	go func() {
		close(disableStarted)
		disableDone <- s.Reload(Config{Enable: false})
	}()
	<-disableStarted
	time.Sleep(50 * time.Millisecond) // 给禁用配置足够时间抢在慢配置发布之前

	close(release)
	if err := <-slowDone; err != nil {
		t.Fatalf("slow reload: %v", err)
	}
	if err := <-disableDone; err != nil {
		t.Fatalf("disable reload: %v", err)
	}

	if s.Ready() {
		t.Error("oidc should stay disabled, a slower reload that started earlier must not resurrect it")
	}
}

// 登出地址必须把 id_token 带给 IdP，否则 IdP 不会结束会话，用户点了退出只是回到夜莺登录页
func TestSsoLogoutAddrCarriesIdToken(t *testing.T) {
	s := &SsoClient{}

	cases := []struct {
		name       string
		logoutAddr string
		idToken    string
		want       string
	}{
		{
			name:       "配置写了模板变量就按模板替换",
			logoutAddr: "https://idp.example.com/logout?id_token_hint={{$__id_token__}}",
			idToken:    "tok",
			want:       "https://idp.example.com/logout?id_token_hint=tok",
		},
		{
			name:       "没写模板变量时补上 id_token_hint",
			logoutAddr: "http://127.0.0.1:19001/logout",
			idToken:    "tok",
			want:       "http://127.0.0.1:19001/logout?id_token_hint=tok",
		},
		{
			name:       "已有的查询参数要保留",
			logoutAddr: "http://127.0.0.1:19001/logout?post_logout_redirect_uri=http%3A%2F%2Fn9e.com%2Flogin",
			idToken:    "tok",
			want:       "http://127.0.0.1:19001/logout?id_token_hint=tok&post_logout_redirect_uri=http%3A%2F%2Fn9e.com%2Flogin",
		},
		{
			name:       "已经带了 id_token_hint 就不动",
			logoutAddr: "http://127.0.0.1:19001/logout?id_token_hint=other",
			idToken:    "tok",
			want:       "http://127.0.0.1:19001/logout?id_token_hint=other",
		},
		{
			name:       "拿不到 id_token 时保持原样",
			logoutAddr: "http://127.0.0.1:19001/logout",
			want:       "http://127.0.0.1:19001/logout",
		},
		{
			name:    "没配登出地址就不要拼出一个相对地址",
			idToken: "tok",
			want:    "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.replaceIdTokenTemplate(tc.logoutAddr, tc.idToken); got != tc.want {
				t.Errorf("logout addr = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNewDisabled(t *testing.T) {
	s, err := New(Config{Enable: false, SsoAddr: deadIdPAddr(t)})
	if err != nil {
		t.Fatalf("disabled oidc should not error: %v", err)
	}
	if s.Ready() {
		t.Error("disabled oidc must not report ready")
	}
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
