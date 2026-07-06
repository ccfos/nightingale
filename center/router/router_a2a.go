package router

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/a2a"
	a2ataskstore "github.com/ccfos/nightingale/v6/aiagent/a2a/taskstore"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/logger"
)

// a2aLogBodyLimit caps how much of the request body we capture for INFO logs.
// A2A JSON-RPC payloads are normally well under 4KB; the cap exists so a
// pathological caller cannot blow up the log file by uploading megabytes.
const a2aLogBodyLimit = 4 * 1024

// a2aLogRespHeadLimit / a2aLogRespTailLimit bound how much of the response we
// echo into the per-request DEBUG "resp" log. A unary reply (tasks/get, a small message)
// fits entirely in the head and is logged verbatim. A streaming (SSE) reply
// can be far larger than the answer it carries — the observed 93KB stream
// assembled to a ~5KB message — so we keep the head (start of the answer) AND
// the tail (the terminal event, e.g. TASK_STATE_COMPLETED) and elide the
// middle. That way the log proves both that the server emitted real content
// and that it reached a terminal state, which is exactly what's needed to tell
// a server-side hang apart from a client that failed to render a complete
// response. bytes_out still reports the full wire size.
const (
	a2aLogRespHeadLimit = 16 * 1024
	a2aLogRespTailLimit = 16 * 1024
)

// a2aMountPrefix is the API path prefix under which the built-in OAuth 2.1
// Authorization Server is mirrored (in addition to the root mount). Deployments
// that reverse-proxy only /api/n9e/* to n9e then reach the AS without any extra
// nginx rule, while clients deriving the metadata location from a root issuer
// still hit the root copy.
const a2aMountPrefix = "/api/n9e"

// a2aBackend adapts *Router into the narrow surface expected by aiagent/a2a.
// Keeps the package boundary clean: aiagent/a2a never imports center/router.
type a2aBackend struct {
	rt *Router
}

func (b *a2aBackend) EnsureAssistantChat(userID int64, chatID string, page models.AssistantPageInfo) (*models.AssistantChat, error) {
	return b.rt.EnsureAssistantChat(userID, chatID, page)
}

func (b *a2aBackend) StartAssistantMessage(userID int64, chat *models.AssistantChat, query models.AssistantMessageQuery, lang string) (*a2a.MessageStartResult, int, error) {
	res, status, err := b.rt.StartAssistantMessage(userID, chat, query, lang)
	if err != nil {
		return nil, status, err
	}
	return &a2a.MessageStartResult{
		ChatID:   res.ChatID,
		SeqID:    res.SeqID,
		StreamID: res.StreamID,
	}, 0, nil
}

func (b *a2aBackend) CancelAssistantMessage(ctx context.Context, chatID string, seqID int64) error {
	// Translate the router-side typed sentinel into the A2A SDK's canonical
	// "task not found" error at the package boundary, so aiagent/a2a never has
	// to know about router internals. Other errors propagate untouched.
	if err := b.rt.CancelAssistantMessageInternal(ctx, chatID, seqID); err != nil {
		if errors.Is(err, ErrMessageNotInflight) {
			return a2asdk.ErrTaskNotFound
		}
		return err
	}
	return nil
}

func (b *a2aBackend) CheckChatOwner(chatID string, userID int64) error {
	_, err := models.AssistantChatCheckOwner(b.rt.Ctx, chatID, userID)
	return err
}

func (b *a2aBackend) StreamBus() aiagent.StreamBus {
	return b.rt.streamBus
}

func (b *a2aBackend) MessageSnapshot(ctx context.Context, chatID string, seqID int64) (*models.AssistantMessage, error) {
	return models.MsgStateGet(ctx, b.rt.Redis, chatID, seqID)
}

// configRegisterA2A mounts the AgentCard, A2A and MCP endpoints. The HTTP path
// "/.well-known/agent.json" is reserved for AgentCard discovery; A2A is mounted
// at /a2a, MCP at /mcp. Both endpoints reuse the n9e tokenAuth middleware.
func (rt *Router) configRegisterA2A(r *gin.Engine) {
	if rt.HTTP.A2A.Disable {
		return
	}
	if !rt.HTTP.TokenAuth.Enable {
		logger.Warning("[A2A] HTTP.TokenAuth.Enable=false — AgentCard advertises X-User-Token apiKey but the server will only accept JWT credentials. Enable HTTP.TokenAuth so the advertised auth scheme actually works.")
	}
	if rt.HTTP.RSAuth.Enable {
		if rt.HTTP.RSAuth.Audience == "" {
			logger.Warning("[A2A] HTTP.RSAuth.Enable=true but HTTP.RSAuth.Audience is empty — OAuth access tokens are rejected until you set Audience to this service's resource identifier.")
		}
		if rt.rsAuthProvider() == "oauth2" {
			switch {
			case rt.Sso == nil || rt.Sso.OAuth2 == nil || !rt.Sso.OAuth2.Enable:
				logger.Warning("[A2A] HTTP.RSAuth.Enable=true with Provider=oauth2 but OAuth2 login is disabled — enable the OAuth2 SSO config for the trusted IdP.")
			case rt.Sso.OAuth2.RSVerifyMethod == "introspect":
				if rt.Sso.OAuth2.IntrospectAddr == "" {
					logger.Warning("[A2A] HTTP.RSAuth.Enable=true with Provider=oauth2 RSVerifyMethod=introspect but its IntrospectAddr is empty — set the RFC 7662 introspection endpoint (or leave RSVerifyMethod empty to use userinfo).")
				}
			default: // "" (default) or "userinfo"
				if rt.Sso.OAuth2.UserInfoAddr == "" {
					logger.Warning("[A2A] HTTP.RSAuth.Enable=true with Provider=oauth2 (userinfo mode) but OAuth2 UserInfoAddr is empty — set UserInfoAddr so opaque tokens can be validated.")
				} else {
					logger.Warning("[A2A] HTTP.RSAuth Provider=oauth2 is in userinfo mode — token audience is NOT enforced (the UserInfo response carries no aud), so any valid token from this IdP is accepted. Set RSVerifyMethod=introspect (RFC 7662) when stronger assurance is required.")
				}
			}
		} else if rt.Sso == nil || rt.Sso.OIDC == nil || !rt.Sso.OIDC.Enable {
			logger.Warning("[A2A] HTTP.RSAuth.Enable=true but OIDC login is not enabled — RSAuth reuses the OIDC provider's JWKS to verify tokens, so enable OIDC for the trusted IdP.")
		}
	}

	backend := &a2aBackend{rt: rt}

	tokenHeader := rt.HTTP.TokenAuth.HeaderUserTokenKey
	if tokenHeader == "" {
		tokenHeader = DefaultTokenKey
	}

	// Build a Redis-backed TaskStore so tasks/get and tasks/resubscribe
	// survive process restarts and LB-induced instance switches. All n9e
	// center instances share the same Redis, so multi-instance correctness
	// comes for free via the store's Lua-CAS update path.
	taskStore := a2ataskstore.NewRedisStore(rt.Redis, a2ataskstore.Options{
		User: func(ctx context.Context) (string, error) {
			user := a2a.UserFromContext(ctx)
			if user == nil {
				return "", nil
			}
			return user.Username, nil
		},
	})

	handlerOpts := a2a.HandlerOptions{TaskStore: taskStore}

	// AgentCard is public — it carries no instance-specific secrets, only a
	// description of the agent's capabilities. Spec requires it to be
	// reachable without authentication so clients can discover.
	cardHandler := gin.WrapH(a2a.AgentCardHandler(a2a.AgentCardOptions{
		BaseURL:         rt.HTTP.A2A.BaseURL,
		A2APath:         "/a2a",
		TokenHeaderName: tokenHeader,
		// When RS auth is on, advertise the IdP's OIDC discovery so A2A clients
		// can self-discover the OAuth option. Passed as a resolver so it is
		// evaluated per request — enabling RS/OIDC (or changing the IdP) at
		// runtime reflects in the card without a center restart.
		OIDCDiscoveryURL: rt.oidcDiscoveryURL,
	}))
	// Canonical A2A v0.3+ path; alias kept for older clients.
	r.GET("/.well-known/agent-card.json", cardHandler)
	r.GET("/.well-known/agent.json", cardHandler)
	// RFC 9728 Protected Resource Metadata — public, served only while RS auth
	// is active (else 404). Lets OAuth-aware clients discover the trusted AS.
	// The path-suffixed aliases match the well-known-URI-insertion form
	// (`/.well-known/oauth-protected-resource/a2a`, `.../mcp`) that clients
	// derive from the endpoint they connect to.
	r.GET("/.well-known/oauth-protected-resource", rt.oauthProtectedResource)
	r.GET("/.well-known/oauth-protected-resource/a2a", rt.oauthProtectedResource)
	r.GET("/.well-known/oauth-protected-resource/mcp", rt.oauthProtectedResource)

	// Built-in OAuth 2.1 Authorization Server endpoints (router_mcp_oauth.go).
	// Public by design — the AS metadata, DCR, authorize and token endpoints are
	// reached before any n9e credential exists; each handler 404s / errors when
	// MCPAuth.Enable is off. The interactive consent screen is a frontend SPA
	// route (/oauth-consent) the authorize endpoint redirects to; the SPA then
	// calls the protected decision API registered in router.go.
	r.GET(mcpASMetaPath, rt.MCPOAuthServerMetadata)
	r.GET(mcpASMetaPath+"/a2a", rt.MCPOAuthServerMetadata)
	r.GET(mcpASMetaPath+"/mcp", rt.MCPOAuthServerMetadata)
	r.POST(mcpRegisterPath, rt.MCPOAuthRegister)
	r.GET(mcpAuthorizePath, rt.MCPOAuthAuthorize)
	r.POST(mcpTokenPath, rt.MCPOAuthToken)
	r.POST(mcpRevokePath, rt.MCPOAuthRevoke)

	// Mirror the built-in AS (metadata + register/authorize/token/revoke) under
	// the /api/n9e API prefix. withMount tags each request with its prefix so the
	// metadata advertises matching /api/n9e/oauth/* endpoints and a
	// "<base>/api/n9e" issuer (see mcpAPIBaseURL); the root copies above keep
	// emitting root URLs. Only the AS is mirrored — the A2A/MCP protocol groups
	// stay root-only because /api/n9e/mcp/*proxyPath would collide with the
	// existing /api/n9e/mcp/oauth/authorize decision route (gin catch-all vs.
	// static child).
	r.GET(a2aMountPrefix+mcpASMetaPath, rt.withMount(a2aMountPrefix, rt.MCPOAuthServerMetadata))
	r.GET(a2aMountPrefix+mcpASMetaPath+"/a2a", rt.withMount(a2aMountPrefix, rt.MCPOAuthServerMetadata))
	r.POST(a2aMountPrefix+mcpRegisterPath, rt.withMount(a2aMountPrefix, rt.MCPOAuthRegister))
	r.GET(a2aMountPrefix+mcpAuthorizePath, rt.withMount(a2aMountPrefix, rt.MCPOAuthAuthorize))
	r.POST(a2aMountPrefix+mcpTokenPath, rt.withMount(a2aMountPrefix, rt.MCPOAuthToken))
	r.POST(a2aMountPrefix+mcpRevokePath, rt.withMount(a2aMountPrefix, rt.MCPOAuthRevoke))
	// RFC 8414 well-known-URI insertion: a client given the prefixed issuer
	// "<base>/api/n9e" looks for the metadata at
	// "<base>/.well-known/oauth-authorization-server/api/n9e" (inserted at root,
	// not appended). Serve that alias, tagged with the prefix so it returns the
	// same prefixed-issuer document.
	r.GET(mcpASMetaPath+a2aMountPrefix, rt.withMount(a2aMountPrefix, rt.MCPOAuthServerMetadata))

	// The SDK's internal http.ServeMux is registered at root paths like
	// /message:send /message:stream /tasks/{id}, so we MUST strip the /a2a
	// mount prefix before delegating; otherwise the SDK's mux sees
	// /a2a/message:send and 404s.
	a2aHandler := http.StripPrefix("/a2a", a2a.NewHTTPHandler(backend, handlerOpts))
	a2aGroup := r.Group("/a2a")
	// requestLog runs first so auth failures (which short-circuit tokenAuth)
	// still produce a structured "[A2A] start/done" pair carrying trace_id.
	a2aGroup.Use(rt.a2aRequestLog("A2A"), rt.rsAuthChallenge(), rt.agentOAuthScope(), rt.tokenAuth(), rt.user(), rt.injectA2AUser(), rt.streamingDeadline())
	// The A2A REST binding mounts every method under a sub-path
	// (/a2a/message:send, /a2a/message:stream, /a2a/tasks/{id}, ...); the bare
	// root has no method. Delegating the root to the SDK would StripPrefix it to
	// "" and let net/http's ServeMux 301-redirect to "/", landing a misconfigured
	// caller (e.g. a legacy JSON-RPC "tasks/send" client posting to the base URL)
	// on the frontend SPA — an HTML body it can't parse, which looks like a hang.
	// Answer the root with an explicit JSON hint instead.
	a2aGroup.Any("", rt.a2aRootEndpointHint)
	a2aGroup.Any("/*proxyPath", gin.WrapH(a2aHandler))

	if !rt.HTTP.A2A.DisableMCP {
		// Pass the engine r as the in-process dispatcher: MCP tool calls are
		// re-dispatched onto /api/n9e/... through the full middleware chain,
		// carrying the caller's X-User-Token so RBAC applies unchanged.
		mcpHandler := http.StripPrefix("/mcp", a2a.NewMCPHandler(r, a2a.MCPConfig{
			Toolsets:      rt.HTTP.A2A.MCPToolsets,
			ReadOnly:      rt.HTTP.A2A.MCPReadOnly,
			TokenHeader:   tokenHeader,
			ExtraToolsets: rt.MCPExtraToolsets,
		}))
		mcpGroup := r.Group("/mcp")
		// /mcp accepts TokenAuth (X-User-Token) and OAuth access tokens, with the
		// same middleware chain as /a2a: rsAuthChallenge attaches the RFC 9728
		// WWW-Authenticate discovery pointer on 401s, agentOAuthScope marks the
		// request as agent-plane so tokenAuth accepts OAuth tokens (builtin AS or
		// external IdP). MCP tool calls replay the caller's raw credential onto
		// the internal /api/n9e hop — TokenAuth tokens under the configured
		// header, Bearer tokens under Authorization plus a2a's in-process
		// dispatch marker that lets the internal tokenAuth treat that hop as
		// agent-plane too (see a2a.IsMCPInProcDispatch).
		mcpGroup.Use(rt.a2aRequestLog("MCP"), rt.rsAuthChallenge(), rt.agentOAuthScope(), rt.tokenAuth(), rt.user(), rt.injectA2AUser(), rt.streamingDeadline())
		mcpGroup.Any("", gin.WrapH(mcpHandler))
		mcpGroup.Any("/*proxyPath", gin.WrapH(mcpHandler))
	}
}

// a2aRequestLog emits an INFO line at request entry (with captured body) and a
// matching one at exit (with status, latency, response byte count). The captured
// response body rides a separate DEBUG line (correlated by trace_id) so it stays
// out of normal INFO logs. scope is "A2A" or "MCP" so the two surfaces can be
// grepped apart. trace_id is taken from the global traceIdMid (pkg/httpx) so the
// lines stitch together with the rest of the access log.
//
// The body is read into memory up to a2aLogBodyLimit and then restored via a
// MultiReader so the downstream SDK still sees the full payload. Non-JSON
// content types are skipped to avoid binary spam in the log.
func (rt *Router) a2aRequestLog(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetString("trace_id")
		method := c.Request.Method
		path := c.Request.URL.Path
		remote := c.ClientIP()

		body, truncated := captureA2ARequestBody(c.Request)

		logger.Infof("[%s] start trace_id=%s method=%s path=%s remote=%s body_len=%d body_truncated=%v body=%s",
			scope, traceID, method, path, remote, len(body), truncated, body)

		// Tee the response so the DEBUG "resp" line can show what was actually
		// returned to the caller. Installed before c.Next() so every downstream
		// write — including streaming SSE frames and auth-failure bodies — flows
		// through it. Only Write/WriteString are intercepted; Flush, Hijack,
		// Status, Size, etc. are promoted unchanged so SSE streaming and the
		// accurate bytes_out below keep working.
		respCap := &a2aResponseCapture{ResponseWriter: c.Writer}
		c.Writer = respCap

		start := time.Now()
		c.Next()
		cost := time.Since(start)

		username := ""
		if v, ok := c.Get("user"); ok {
			if u, _ := v.(*models.User); u != nil {
				username = u.Username
			}
		}

		logger.Infof("[%s] done trace_id=%s method=%s path=%s user=%s status=%d cost=%s bytes_out=%d",
			scope, traceID, method, path, username, c.Writer.Status(), cost, c.Writer.Size())

		// The captured response body is large (a streaming SSE reply assembles to
		// tens of KB) and may carry user content, so keep it off the operational
		// INFO line and emit it only at DEBUG; trace_id ties it back to the pair.
		resp, respTruncated := respCap.preview()
		logger.Debugf("[%s] resp trace_id=%s path=%s resp_truncated=%v resp=%s",
			scope, traceID, path, respTruncated, resp)
	}
}

// a2aResponseCapture wraps gin.ResponseWriter to tee the response body into a
// bounded in-memory view for logging (see a2aLogRespHeadLimit /
// a2aLogRespTailLimit). It keeps the first head bytes and the last tail bytes
// so both the start of the answer and the terminal SSE event survive even when
// the full stream is much larger. Every method other than Write/WriteString is
// promoted from the embedded writer, so Flush (SSE), Hijack, Status, Size and
// friends behave exactly as before.
type a2aResponseCapture struct {
	gin.ResponseWriter
	head  []byte
	tail  []byte
	total int
}

func (w *a2aResponseCapture) capture(b []byte) {
	w.total += len(b)
	if room := a2aLogRespHeadLimit - len(w.head); room > 0 {
		n := len(b)
		if n > room {
			n = room
		}
		w.head = append(w.head, b[:n]...)
	}
	w.tail = append(w.tail, b...)
	if over := len(w.tail) - a2aLogRespTailLimit; over > 0 {
		w.tail = w.tail[over:]
	}
}

func (w *a2aResponseCapture) Write(b []byte) (int, error) {
	w.capture(b)
	return w.ResponseWriter.Write(b)
}

func (w *a2aResponseCapture) WriteString(s string) (int, error) {
	w.capture([]byte(s))
	return w.ResponseWriter.WriteString(s)
}

// Unwrap exposes the wrapped writer so http.ResponseController (used by
// clearWriteDeadline on streaming endpoints) can traverse past this wrapper to
// the underlying connection. gin.ResponseWriter the interface does not declare
// Unwrap, so embedding it does not promote one — without this method the
// controller hits its default branch, SetWriteDeadline silently no-ops, and the
// 40s WriteTimeout it was meant to clear stays armed, cutting off long agent
// turns / SSE streams.
func (w *a2aResponseCapture) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// preview reconstructs a bounded view of the captured response. When the body
// fits within the head+tail budget it is returned in full (the second branch
// stitches head and tail back together with no overlap); a larger body returns
// head + a "<N bytes omitted>" marker + tail. The bool reports whether any
// bytes were omitted.
func (w *a2aResponseCapture) preview() (string, bool) {
	if w.total <= len(w.head) {
		return string(w.head), false
	}
	if w.total <= a2aLogRespHeadLimit+a2aLogRespTailLimit {
		// No gap between head and tail: the bytes after the head all still live
		// in the tail ring, so drop the part of the tail already covered by the
		// head and the concatenation is the exact, complete body.
		overlap := len(w.tail) - (w.total - len(w.head))
		return string(w.head) + string(w.tail[overlap:]), false
	}
	omitted := w.total - len(w.head) - len(w.tail)
	return fmt.Sprintf("%s\n...<%d bytes omitted>...\n%s", w.head, omitted, w.tail), true
}

// captureA2ARequestBody reads up to a2aLogBodyLimit bytes from r.Body for
// logging, then restores r.Body so the downstream handler sees the full
// original payload. Returns (preview, truncated). Non-JSON Content-Types are
// skipped (preview = "<skipped non-json content-type=...>") to keep the log
// readable when callers POST binary or multipart payloads.
func captureA2ARequestBody(r *http.Request) (string, bool) {
	if r == nil || r.Body == nil || r.Body == http.NoBody {
		return "", false
	}
	ct := r.Header.Get("Content-Type")
	if ct != "" && !strings.Contains(ct, "json") {
		return "<skipped non-json content-type=" + ct + ">", false
	}

	// Read one byte past the limit so we can tell "exactly at limit" from
	// "truncated" without an extra Read after the LimitedReader.
	limited := io.LimitReader(r.Body, a2aLogBodyLimit+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		// Restore what we managed to read so the downstream still has a
		// chance; surface the read failure in the preview so it's visible.
		r.Body = io.NopCloser(bytes.NewReader(buf))
		return "<read-error: " + err.Error() + ">", false
	}

	truncated := len(buf) > a2aLogBodyLimit
	preview := buf
	if truncated {
		preview = buf[:a2aLogBodyLimit]
		// Stitch what we already read back together with the unread tail so
		// the downstream handler sees the original byte stream.
		r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(buf), r.Body))
	} else {
		r.Body = io.NopCloser(bytes.NewReader(buf))
	}
	return string(preview), truncated
}

// injectA2AUser pulls *models.User from gin.Context (set by rt.user()) and
// stuffs it into request.Context so the a2a executor / mcp handler can read
// it without depending on gin. The streaming write-deadline relaxation is
// applied separately via streamingDeadline() so each middleware does one
// thing.
func (rt *Router) injectA2AUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetString("trace_id")
		v, ok := c.Get("user")
		if !ok {
			logger.Warningf("[A2A] injectA2AUser trace_id=%s path=%s: no user in context — upstream auth middleware did not set one",
				traceID, c.Request.URL.Path)
			c.Abort()
			ginx.Bomb(http.StatusUnauthorized, "unauthorized")
			return
		}
		// A bad type assertion here means rt.user() ran but stuffed something
		// other than *models.User into the context — that's a middleware-chain
		// wiring bug, not a credential problem. Surface it as 500 so it can't
		// be confused with normal auth failures during incident triage.
		user, ok := v.(*models.User)
		if !ok || user == nil {
			logger.Errorf("[A2A] injectA2AUser trace_id=%s path=%s: user middleware returned wrong type %T",
				traceID, c.Request.URL.Path, v)
			c.Abort()
			ginx.Bomb(http.StatusInternalServerError, "a2a: user middleware returned wrong type")
			return
		}
		c.Request = c.Request.WithContext(a2a.WithUser(c.Request.Context(), user))
		c.Next()
	}
}

// streamingDeadline relaxes the per-connection write deadline (default
// http.Server.WriteTimeout, 40s) ONLY for endpoints whose response can
// legitimately outlive it:
//
//   - /a2a/message:send         runs a full agent turn synchronously before
//                               writing the final message (can take minutes)
//   - /a2a/message:stream       SSE; silent for minutes between token bursts
//   - /a2a/tasks/{id}:subscribe SSE task event stream
//   - /mcp and /mcp/*           the Streamable HTTP transport answers POST/GET
//                               on the root with a long-lived text/event-stream
//
// Every other endpoint (the bare /a2a root, tasks get/list/cancel, push-config
// CRUD) is a fast request/response and so KEEPS the WriteTimeout backstop. That
// way a misrouted or stuck write on those paths can't pin a handler goroutine
// forever — which is what happened when a bare POST /a2a leaked a goroutine
// after this middleware used to blanket-clear the deadline for the whole group.
func (rt *Router) streamingDeadline() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a2aWriteDeadlineExempt(c.Request.URL.Path) {
			clearWriteDeadline(c.Writer)
		}
		c.Next()
	}
}

// a2aWriteDeadlineExempt reports whether path serves a streaming or
// long-running-agent response that must not be cut off by http.Server's
// WriteTimeout. Kept as a pure path test so it stays trivially unit-testable.
func a2aWriteDeadlineExempt(path string) bool {
	switch {
	case path == "/mcp" || strings.HasPrefix(path, "/mcp/"):
		// MCP multiplexes unary + streaming over the root; the transport decides
		// per request, so the whole group needs the relaxed deadline.
		return true
	case path == "/a2a/message:send" || path == "/a2a/message:stream":
		return true
	case strings.HasPrefix(path, "/a2a/tasks/") && strings.HasSuffix(path, ":subscribe"):
		return true
	default:
		return false
	}
}

// a2aRootEndpointHint answers the method-less root /a2a path with 404 + a JSON
// body pointing the caller at the real A2A v0.3 REST endpoints, instead of the
// SDK's 301-to-SPA. Auth still runs first (this is the group's final handler),
// so only authenticated-but-misconfigured callers reach it.
func (rt *Router) a2aRootEndpointHint(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{
		"error": "no A2A method at /a2a; this server speaks the A2A v0.3 HTTP+JSON (REST) binding, not legacy JSON-RPC (tasks/send)",
		"endpoints": gin.H{
			"send_message":   "POST /a2a/message:send",
			"stream_message": "POST /a2a/message:stream",
			"get_task":       "GET /a2a/tasks/{id}",
			"cancel_task":    "POST /a2a/tasks/{id}:cancel",
		},
		"agent_card": "/.well-known/agent-card.json",
	})
}
