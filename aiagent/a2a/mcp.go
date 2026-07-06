package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/ccfos/nightingale/v6/pkg/logx"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	mcpapi "github.com/n9e/n9e-mcp-server/pkg/api"
	mcpclient "github.com/n9e/n9e-mcp-server/pkg/client"
	mcptoolset "github.com/n9e/n9e-mcp-server/pkg/toolset"
)

const (
	// mcpUserTokenHeader is the header n9e-mcp-server's client hardcodes on its
	// outbound API requests, and the default header the center authenticates
	// with. When the center is configured with a different TokenAuth header
	// (MCPConfig.TokenHeader), the in-process hop replays the token under that
	// header too — see inProcRoundTripper.
	mcpUserTokenHeader = "X-User-Token"

	// mcpInternalBaseURL is a placeholder base for the in-process transport. The
	// RoundTripper never dials, so only the request path/method/headers matter;
	// the host is meaningless.
	mcpInternalBaseURL = "http://127.0.0.1"

	mcpUserAgent = "nightingale-center-mcp"

	mcpInstructions = "Nightingale (n9e) monitoring MCP Server. Provides alert rule management, " +
		"active/history alert querying, alert mute/silence management, notification rules, " +
		"alert subscriptions, user/team management, monitored target management, " +
		"datasource management, business group management, and event pipeline/workflow management."
)

// All of n9e-mcp-server's toolsets work through the center's in-process
// transport, including metrics: its tools decode the native Prometheus
// envelope the /api/n9e/proxy route forwards (doPromGet in n9e-mcp-server),
// so the empty-whitelist default is simply every default toolset.

// MCPToolsetRegistrar registers extra toolsets onto the /mcp tool group.
// Embedders (e.g. the enterprise edition) use it to expose their own API
// surface as MCP tools alongside the default n9e-mcp-server toolsets, without
// this package knowing about them. The tools' outbound requests go through
// the same in-process transport, so the embedder's routes see the caller's
// credential and their own auth/RBAC/license middlewares apply unchanged.
// getClient resolves the caller-scoped client from a tool call's context.
type MCPToolsetRegistrar func(group *mcptoolset.ToolsetGroup, getClient mcpclient.GetClientFunc)

// MCPConfig selects which toolsets /mcp exposes and how the caller's
// credential is replayed onto the internal API hop.
type MCPConfig struct {
	// Toolsets is the enabled toolset whitelist; empty means everything
	// registered (default toolsets plus ExtraToolsets). Unknown names are
	// dropped (see resolveToolsets), never widened to all.
	Toolsets []string
	// ReadOnly registers only read tools when true.
	ReadOnly bool
	// TokenHeader is the center's configured TokenAuth header name (defaults to
	// X-User-Token). The caller's token is read from and replayed under it.
	TokenHeader string
	// ExtraToolsets lets an embedder register additional toolsets on top of
	// n9e-mcp-server's defaults; run in order after the defaults.
	ExtraToolsets []MCPToolsetRegistrar
}

// NewMCPHandler builds an MCP Streamable HTTP handler that exposes the
// n9e-mcp-server toolset (~60 fine-grained tools across 13 toolsets), running
// entirely inside the center process. Mount it under a gin group that applies
// the n9e tokenAuth middleware.
//
// Each tool call is dispatched back into dispatcher (the center gin engine) as
// an in-process HTTP request against /api/n9e/..., carrying the caller's
// TokenAuth token so RBAC and business-group permissions apply exactly as for a
// real API call — the tool table and behaviour stay identical to the standalone
// n9e-mcp-server. dispatcher is typed as http.Handler (not *gin.Engine) so this
// package does not depend on center/router.
//
// Callers authenticate with either the TokenAuth header (X-User-Token) or an
// OAuth access token (Authorization: Bearer — builtin AS or external IdP; the
// same credentials the /mcp gin group's tokenAuth accepts at the edge). The
// raw credential is replayed onto the internal hop as-is: TokenAuth tokens
// under the configured header, Bearer tokens under Authorization. The internal
// tokenAuth normally rejects OAuth tokens outside the agent plane; it makes an
// exception for requests carrying the in-process dispatch marker
// (IsMCPInProcDispatch), which only this package can attach. The
// credential-presence check below is a defence-in-depth backstop for any
// caller that still authenticates without a replayable header credential
// (e.g. a session cookie): it returns a clear 401 rather than a toolset whose
// every call fails.
func NewMCPHandler(dispatcher http.Handler, cfg MCPConfig) http.Handler {
	tokenHeader := cfg.TokenHeader
	if tokenHeader == "" {
		tokenHeader = mcpUserTokenHeader
	}

	// One shared *http.Client whose transport dispatches in-process. The caller's
	// token rides per-request in the header the client sets outbound, so a single
	// transport is safe to reuse.
	httpClient := &http.Client{Transport: &inProcRoundTripper{dispatcher: dispatcher, tokenHeader: tokenHeader}}

	// A single shared server: the tools are registered once, and the
	// per-caller n9e client is built on demand from the token in the request
	// context (stashed below). There is deliberately no per-token server cache —
	// such a cache never evicts and would grow unbounded as tokens rotate.
	server := buildMCPServer(httpClient, cfg)
	streamable := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true},
	)

	authErrBody, _ := json.Marshal(map[string]string{
		"error": "nightingale MCP requires " + tokenHeader + " or OAuth Bearer (Authorization header) authentication",
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TokenAuth header first (existing semantics), Bearer as the OAuth path.
		cred := mcpCredential{token: strings.TrimSpace(r.Header.Get(tokenHeader))}
		if cred.token == "" {
			if raw := strings.TrimSpace(r.Header.Get("Authorization")); len(raw) > 7 && strings.EqualFold(raw[:7], "Bearer ") {
				cred = mcpCredential{token: strings.TrimSpace(raw[7:]), bearer: true}
			}
		}
		if cred.token == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write(authErrBody)
			return
		}
		// Stash the caller credential so the shared server's receiving middleware
		// can build a caller-scoped client for this request without any cache. The
		// SDK threads req.Context() through to that middleware (via
		// server.Connect) and onward into each tool's outbound API request.
		r = r.WithContext(context.WithValue(r.Context(), mcpTokenCtxKey{}, cred))
		streamable.ServeHTTP(w, r)
	})
}

// resolveToolsets returns the enabled-toolset list. available is what is
// actually registered on the group (defaults plus any embedder extras). An
// empty whitelist (and the explicit "all" keyword, kept for config
// back-compat) means everything available. Otherwise only the known names are
// kept: an unknown name is dropped with a warning — a typo shrinks the
// exposed set, it never fails open by widening a restricted whitelist to all
// toolsets.
func resolveToolsets(cfg MCPConfig, available []string) []string {
	// Sort a copy: callers may hand in a shared slice (e.g. a package-level
	// default list) that must not be reordered in place.
	available = append([]string(nil), available...)
	sort.Strings(available)
	if len(cfg.Toolsets) == 0 {
		return available
	}

	valid := make(map[string]bool, len(available))
	for _, name := range available {
		valid[name] = true
	}

	enabled := make([]string, 0, len(cfg.Toolsets))
	for _, raw := range cfg.Toolsets {
		name := strings.TrimSpace(raw)
		switch {
		case name == "":
			continue
		case name == "all":
			return available
		case !valid[name]:
			logx.Warningf(context.Background(), "[MCP] ignoring unknown toolset %q; valid names: %v", name, available)
		default:
			enabled = append(enabled, name)
		}
	}
	return enabled
}

// mcpCredential is the caller's replayable credential, carried from the /mcp
// HTTP entrypoint (NewMCPHandler stashes it per request) to the shared
// server's receiving middleware and the in-process RoundTripper. bearer marks
// a credential that arrived as "Authorization: Bearer" (OAuth access token or
// JWT) and must be replayed under that header on the internal hop, rather
// than as an X-User-Token.
type mcpCredential struct {
	token  string
	bearer bool
}

// mcpTokenCtxKey is the context key under which the mcpCredential travels.
type mcpTokenCtxKey struct{}

// IsMCPInProcDispatch reports whether ctx belongs to the center's in-process
// /mcp tool dispatch — i.e. the request entered through an authenticated /mcp
// call and its credential is being replayed onto the internal /api/n9e hop.
// The center's tokenAuth uses this as the agent-plane marker to accept OAuth
// access tokens on that hop, which it otherwise only does under the /a2a and
// /mcp gin groups. The key type is unexported, so nothing outside this
// package can forge the marker onto an external request.
func IsMCPInProcDispatch(ctx context.Context) bool {
	_, ok := ctx.Value(mcpTokenCtxKey{}).(mcpCredential)
	return ok
}

// buildMCPServer assembles the single shared mcp.Server. Tools speak to the
// center via a token-scoped in-process client that the receiving middleware
// builds on demand from the token in the request context — no per-token
// server or client cache to grow unbounded as tokens rotate.
func buildMCPServer(httpClient *http.Client, cfg MCPConfig) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "Nightingale MCP Server",
		Version: "1.0.0",
	}, &mcp.ServerOptions{
		Instructions: mcpInstructions,
	})

	// Build the caller-scoped client per request and inject it so every tool
	// handler resolves it from ctx, exactly as the standalone n9e-mcp-server
	// does. The token was stashed by NewMCPHandler; the SDK threads the HTTP
	// request context through to this middleware.
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			cred, _ := ctx.Value(mcpTokenCtxKey{}).(mcpCredential)
			// Inputs are fixed and valid (non-nil client, constant base URL), so
			// this cannot error.
			n9eClient, _ := mcpclient.NewClientWithHTTPClient(cred.token, mcpInternalBaseURL, mcpUserAgent, httpClient)
			return next(mcpclient.ContextWithClient(ctx, n9eClient), method, req)
		}
	})

	group := mcpapi.DefaultToolsetGroup(mcpclient.ClientFromContext, cfg.ReadOnly)
	for _, register := range cfg.ExtraToolsets {
		if register != nil {
			register(group, mcpclient.ClientFromContext)
		}
	}
	// The whitelist is validated against the composed group, so this cannot
	// error on unknown names.
	_ = group.EnableToolsets(resolveToolsets(cfg, group.GetAvailableToolsets()))
	group.RegisterAll(server)

	return server
}

// inProcRoundTripper turns each outbound tool request into an in-process call on
// the center gin engine, so n9e-mcp-server's tools reach /api/n9e/... without
// opening a socket. The request carries the caller's token, so the internal
// tokenAuth chain authenticates the same caller and RBAC applies unchanged.
//
// Re-entrancy is safe: /api/n9e/... and /mcp are distinct routes, so ServeHTTP
// does not recurse, and gin pulls a fresh *gin.Context from its pool per call.
type inProcRoundTripper struct {
	dispatcher  http.Handler
	tokenHeader string
}

func (t *inProcRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// The client builds a fresh *http.Request per call and never reuses it, so
	// adjusting it here is safe.
	if req.RemoteAddr == "" {
		req.RemoteAddr = "127.0.0.1:0"
	}
	// n9e-mcp-server's client hardcodes the credential under X-User-Token. A
	// Bearer (OAuth) caller's credential must instead reach the internal
	// tokenAuth as "Authorization: Bearer": move it there and drop the
	// X-User-Token stamp. The internal tokenAuth accepts OAuth tokens on this
	// hop because req.Context() carries the in-process dispatch marker (see
	// IsMCPInProcDispatch) — the same context NewMCPHandler stashed the
	// credential into, threaded here through the SDK and the client's
	// NewRequestWithContext.
	if cred, ok := req.Context().Value(mcpTokenCtxKey{}).(mcpCredential); ok && cred.bearer {
		req.Header.Del(mcpUserTokenHeader)
		req.Header.Set("Authorization", "Bearer "+cred.token)
	} else if t.tokenHeader != "" && t.tokenHeader != mcpUserTokenHeader {
		// TokenAuth caller with a non-default header configured: replay the
		// token there too so the internal tokenAuth sees it. No-op when the
		// headers match.
		if tok := req.Header.Get(mcpUserTokenHeader); tok != "" {
			req.Header.Set(t.tokenHeader, tok)
		}
	}

	rec := &responseCapture{header: make(http.Header)}
	t.dispatcher.ServeHTTP(rec, req)

	status, header, body := rec.status, rec.header, rec.body.Bytes()
	if status == 0 {
		status = http.StatusOK
	}
	if rec.overflow {
		// The internal response blew past the cap. Fail closed with a
		// non-retryable 4xx (a 5xx would make the client re-run the oversized
		// query) and a tiny body, so nothing further is buffered and the tool
		// gets a clear error instead of a truncated payload.
		status = http.StatusRequestEntityTooLarge
		header = http.Header{"Content-Type": {"application/json"}}
		body = []byte(`{"err":"nightingale MCP: internal API response exceeded the 64MiB cap"}`)
	}
	return &http.Response{
		Status:        http.StatusText(status),
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}, nil
}

// mcpMaxInternalResponseBytes bounds how much of an internal /api/n9e response
// the transport buffers in the center's heap. It matches n9e-mcp-server's own
// largest response cap (its dashboardMaxResponseSize, 64 MiB); the client's
// per-request LimitReader only runs after RoundTrip returns, so without this a
// pathologically large query result would buffer unbounded here and could OOM
// the whole center process.
const mcpMaxInternalResponseBytes = 64 << 20 // 64 MiB

// responseCapture is a minimal http.ResponseWriter that buffers the in-process
// response so RoundTrip can hand it back as an *http.Response. It stops
// buffering past mcpMaxInternalResponseBytes and flags overflow so RoundTrip can
// fail the call rather than let the heap grow without bound.
type responseCapture struct {
	header   http.Header
	body     bytes.Buffer
	status   int
	overflow bool
}

func (r *responseCapture) Header() http.Header { return r.header }

func (r *responseCapture) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *responseCapture) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	if r.overflow {
		// Already capped: drop the excess but report success so the underlying
		// handler runs to completion instead of erroring mid-write.
		return len(b), nil
	}
	if room := mcpMaxInternalResponseBytes - r.body.Len(); len(b) > room {
		if room > 0 {
			r.body.Write(b[:room])
		}
		r.overflow = true
		return len(b), nil
	}
	return r.body.Write(b)
}
