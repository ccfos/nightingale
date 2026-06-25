package httpx

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/aop"
	"github.com/ccfos/nightingale/v6/pkg/logx"
	"github.com/ccfos/nightingale/v6/pkg/version"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Config struct {
	Host             string
	Port             int
	CertFile         string
	KeyFile          string
	PProf            bool
	PrintAccessLog   bool
	PrintBody        bool
	ExposeMetrics    bool
	ShutdownTimeout  int
	MaxContentLength int64
	ReadTimeout      int
	WriteTimeout     int
	IdleTimeout      int
	JWTAuth          JWTAuth
	ProxyAuth        ProxyAuth
	ShowCaptcha      ShowCaptcha
	APIForAgent      BasicAuths
	APIForService    BasicAuths
	RSA              RSAConfig
	TokenAuth        TokenAuth
	RSAuth           RSAuth
	MCPAuth          MCPAuth
	A2A              A2AConfig
}

// A2AConfig controls the A2A (Agent-to-Agent) and MCP (Model Context Protocol)
// HTTP endpoints exposed by n9e. Both are enabled by default; auth reuses the
// existing TokenAuth middleware (X-User-Token).
type A2AConfig struct {
	// Disable turns off both the A2A and MCP endpoints when true.
	Disable bool
	// DisableMCP turns off only the MCP endpoint while keeping A2A active.
	DisableMCP bool
	// BaseURL is the absolute URL advertised in the AgentCard. When empty it
	// is derived from the incoming request (Host + X-Forwarded-Proto / TLS).
	BaseURL string
}

type RSAConfig struct {
	OpenRSA           bool
	RSAPublicKey      []byte
	RSAPublicKeyPath  string
	RSAPrivateKey     []byte
	RSAPrivateKeyPath string
	RSAPassWord       string
}

type ShowCaptcha struct {
	Enable bool
}

type BasicAuths struct {
	BasicAuth gin.Accounts
	Enable    bool
}

type ProxyAuth struct {
	Enable            bool
	HeaderUserNameKey string
	DefaultRoles      []string
}

type JWTAuth struct {
	SigningKey     string
	AccessExpired  int64
	RefreshExpired int64
	RedisKeyPrefix string
	SingleLogin    bool
}

type TokenAuth struct {
	Enable             bool
	HeaderUserTokenKey string
}

// RSAuth turns n9e's TokenAuth-protected endpoints (notably the A2A/MCP agent
// endpoints) into an OAuth 2.1 Resource Server: a Bearer access token minted by
// the external IdP already configured for SSO login is accepted as a per-user
// credential. Provider selects how the token is verified — "oidc" validates
// JWTs against the IdP's JWKS, "oauth2" validates opaque tokens via the OAuth2
// SSO config's RSVerifyMethod. Disabled by default; the existing X-User-Token
// and session-JWT paths are unaffected.
type RSAuth struct {
	Enable bool
	// Audience is this service's resource identifier; RS auth stays off while it
	// is empty. NOTE: `aud` is only enforced by the oidc (JWKS) path and the
	// oauth2 introspection path (RSVerifyMethod=introspect). The oauth2 userinfo
	// path — the oauth2 default — cannot read an `aud` and therefore does NOT
	// enforce it: any valid token from the trusted IdP is accepted. Set the
	// OAuth2 SSO config's RSVerifyMethod=introspect when audience binding is
	// required.
	Audience string
	// Provider selects which SSO login provider verifies RS access tokens:
	// "oidc" (default) validates JWTs locally via the IdP's JWKS; "oauth2"
	// validates opaque tokens via the OAuth2 SSO config's RSVerifyMethod
	// (userinfo by default, or RFC 7662 introspection).
	Provider string
}

// MCPAuth turns n9e itself into an OAuth 2.1 Authorization Server (co-located
// with the Resource Server) so generic MCP clients (Claude / ChatGPT connector)
// can connect to /a2a /mcp with zero pre-configuration via RFC 7591 Dynamic
// Client Registration — the "no external IdP" counterpart to RSAuth. Disabled by
// default; orthogonal to RSAuth (both may be enabled, both advertised in the
// RFC 9728 protected-resource metadata). The existing PAT, session-JWT and
// external-IdP RS paths are unaffected.
//
// Design (see doc/api/mcp-oauth-as.md): stateless signed JWTs everywhere — the
// client_id, authorization-request ticket, authorization code, access and
// refresh tokens are all HS256 JWTs distinguished by a token_use claim and
// signed with a key derived from JWTAuth.SigningKey (or SigningKey below),
// cryptographically separate from the session-JWT key. The only shared state is
// a one-time-use guard for authorization codes in the shared Redis, so the AS
// is correct across all center instances behind a load balancer.
type MCPAuth struct {
	// Enable turns the built-in Authorization Server on.
	Enable bool
	// Issuer is this AS's canonical URL (the `iss` of issued tokens and the
	// `issuer` of the RFC 8414 metadata). In multi-instance deployments set it
	// explicitly so every instance advertises an identical issuer regardless of
	// which hostname/proto a request arrives on; left empty it is derived from
	// the request (A2A.BaseURL, else Host + X-Forwarded-Proto / TLS).
	Issuer string
	// Resource is the MCP resource identifier bound into the access token `aud`
	// (RFC 8707). Left empty it falls back to RSAuth.Audience, else to
	// "<base>/mcp". When both MCPAuth and RSAuth are enabled, set this equal to
	// RSAuth.Audience so the two share one resource id.
	Resource string
	// SigningKey, when set, signs all MCP OAuth JWTs. Left empty a 32-byte key is
	// derived from JWTAuth.SigningKey via HKDF-SHA256 (deterministic across
	// instances, independent from the session key). Must be identical on every
	// instance; never auto-generate per process.
	SigningKey string
	// AccessTTL / RefreshTTL / CodeTTL are token lifetimes in seconds; zero
	// values fall back to 3600 / 604800 / 60.
	AccessTTL  int64
	RefreshTTL int64
	CodeTTL    int64
	// RequireConsent, when false, lets the frontend skip the explicit consent
	// screen for an already-logged-in user (still issues the code). Default true.
	RequireConsent bool
}

func GinEngine(mode string, cfg Config, printBodyPaths func() map[string]struct{},
	printAccessLog func() bool) *gin.Engine {
	gin.SetMode(mode)

	loggerMid := aop.Logger(aop.LoggerConfig{PrintAccessLog: printAccessLog,
		PrintBodyPaths: printBodyPaths})
	recoveryMid := aop.Recovery()

	if strings.ToLower(mode) == "release" {
		aop.DisableConsoleColor()
	}

	r := gin.New()

	r.Use(traceIdMid())

	r.Use(recoveryMid)

	r.Use(loggerMid)

	if cfg.PProf {
		pprof.Register(r, "/api/debug/pprof")
	}

	r.GET("/ping", func(c *gin.Context) {
		c.String(200, "pong")
	})

	r.GET("/pid", func(c *gin.Context) {
		c.String(200, fmt.Sprintf("%d", os.Getpid()))
	})

	r.GET("/ppid", func(c *gin.Context) {
		c.String(200, fmt.Sprintf("%d", os.Getppid()))
	})

	r.GET("/addr", func(c *gin.Context) {
		c.String(200, c.Request.RemoteAddr)
	})

	r.GET("/api/n9e/version", func(c *gin.Context) {
		c.String(200, version.Version)
	})

	if cfg.ExposeMetrics {
		r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	}

	return r
}

func traceIdMid() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Trace-Id")
		if !isValidTraceId(id) {
			id = uuid.New().String()
		}
		c.Set("trace_id", id)
		ctx := logx.NewTraceContext(c.Request.Context(), id)
		c.Request = c.Request.WithContext(ctx)
		c.Header("X-Trace-Id", id)
		c.Next()
	}
}

func isValidTraceId(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func Init(cfg Config, handler http.Handler) func() {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  time.Duration(cfg.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.IdleTimeout) * time.Second,
	}

	go func() {
		fmt.Println("http server listening on:", addr)

		var err error
		if cfg.CertFile != "" && cfg.KeyFile != "" {
			srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
			err = srv.ListenAndServeTLS(cfg.CertFile, cfg.KeyFile)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(cfg.ShutdownTimeout))
		defer cancel()

		srv.SetKeepAlivesEnabled(false)
		if err := srv.Shutdown(ctx); err != nil {
			fmt.Println("cannot shutdown http server:", err)
		}

		select {
		case <-ctx.Done():
			fmt.Println("http exiting")
		default:
			fmt.Println("http server stopped")
		}
	}
}
