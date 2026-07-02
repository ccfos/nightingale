package skillgateway

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/logger"
)

// The Skill Gateway is a per-execution HTTP passthrough (§12): it forwards the
// launching user's READ requests to n9e's OWN API (loopback), acting as that
// user, so a skill can survey the platform without n9e enumerating every read
// endpoint. Deny-list security model:
//
//   - GET is allowed by default (deny-list, below). POST is refused wholesale
//     EXCEPT a small allowlist of read-only DATA-query endpoints (postAllowN9eAPIPaths:
//     /ds-query, /query-range-batch, …) that carry a JSON query body — they run
//     under the user's RBAC and return data, not config. PUT/DELETE/PATCH are
//     always refused, so there is no write surface to blacklist.
//   - A path blacklist (built-in defaults + Deny.N9eAPI) refuses the GETs that
//     return secrets (datasource configs, notify-channel secrets, user tokens,
//     SSO configs, datasource proxy). This is fail-OPEN by nature: a read endpoint
//     not on the list is reachable, so the default list must stay comprehensive.
//     A tiny allow-exception set (getAllowExceptions) re-permits specific safe
//     sub-paths shadowed by a broad deny prefix (e.g. the redacted /datasource/brief).
//   - n9e's own route middleware still enforces the user's RBAC + busi-group
//     scope on every forwarded call (the gateway acts strictly AS the user).
//
// Identity is the chat session owner (bound at Start, never supplied by the
// script). The user's API token is resolved host-side and used for the loopback
// auth header — it is NEVER passed into the sandbox (§12.1).

// maxRespBytes caps a forwarded response so a skill can't pull an unbounded
// payload back through the gateway.
const maxRespBytes = 1 << 20 // 1 MiB

// maxReqBodyBytes caps the JSON body a skill may POST to a data-query endpoint,
// so a runaway query payload can't be used to hammer the upstream.
const maxReqBodyBytes = 256 << 10 // 256 KiB

// Bounds on the host-side listener. The peer is the untrusted in-sandbox process:
// gatewayIdleTimeout reclaims a connection held open without sending (slowloris),
// and maxGatewayConns caps host goroutines/fds per execution. The request line
// itself is already bounded by the scanner buffer in handleConn.
const (
	gatewayIdleTimeout = 30 * time.Second
	maxGatewayConns    = 16
)

// defaultTokenHeader is n9e's fixed-token auth header (router_mw.DefaultTokenKey).
const defaultTokenHeader = "X-User-Token"

// Config is the gateway's HTTP-passthrough policy. The deny-list (DenyPaths) is
// the only enumerated surface; reads are otherwise open.
type Config struct {
	BaseURL     string   // loopback base for n9e's own API, e.g. "http://127.0.0.1:17000". Empty = passthrough off.
	TokenHeader string   // fixed-token auth header (default X-User-Token)
	DenyPaths   []string // EXTRA deny path prefixes, merged with the built-in defaults
	RatePerSec  float64
	RateBurst   int
}

// Params binds one gateway instance to one execution.
type Params struct {
	ExecID    string
	SkillName string
	UserID    int64
	DBCtx     *ctx.Context
	Config    Config
	// CacheToken injects a freshly-created user token into the live token cache so
	// it authenticates immediately. Provided by the router (wraps
	// memsto.UserTokenCache.Inject); nil is tolerated (then a new token only works
	// after the next cache sync).
	CacheToken func(token string, user *models.User)
}

type Gateway struct {
	execID  string
	skill   string
	dbctx   *ctx.Context
	user    *models.User
	limiter *tokenBucket

	baseURL     string
	tokenHeader string
	token       string // the launching user's API token, held host-side; never enters the sandbox
	denyPaths   []string
	client      *http.Client

	ln      net.Listener
	path    string
	closed  atomic.Bool
	connSem chan struct{} // per-exec concurrent-connection cap
}

// Start resolves the bound user + a usable API token, binds sockPath (0600), and
// serves. Caller must Close() when the run ends.
func Start(sockPath string, p Params) (*Gateway, error) {
	if p.DBCtx == nil {
		return nil, errors.New("skill-gateway: nil db context")
	}
	if p.UserID <= 0 {
		return nil, errors.New("skill-gateway: no bound user id (identity must come from the chat session owner)")
	}
	if strings.TrimSpace(p.Config.BaseURL) == "" {
		return nil, errors.New("skill-gateway: no n9e api base url configured")
	}
	user, err := models.UserGetById(p.DBCtx, p.UserID)
	if err != nil {
		return nil, fmt.Errorf("skill-gateway: resolve user %d: %w", p.UserID, err)
	}
	if user == nil {
		return nil, fmt.Errorf("skill-gateway: bound user %d not found", p.UserID)
	}

	// Resolve a token to act as this user on n9e's own HTTP API: reuse an existing
	// one (already in the auth cache) or create + cache a fresh one. Held host-side,
	// never handed to the sandbox (§12.1).
	token, err := resolveUserToken(p.DBCtx, user, p.CacheToken)
	if err != nil {
		return nil, fmt.Errorf("skill-gateway: resolve token for %s: %w", user.Username, err)
	}

	_ = os.Remove(sockPath)
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("skill-gateway: listen %s: %w", sockPath, err)
	}
	_ = os.Chmod(sockPath, 0o600)

	hdr := strings.TrimSpace(p.Config.TokenHeader)
	if hdr == "" {
		hdr = defaultTokenHeader
	}

	g := &Gateway{
		execID:      p.ExecID,
		skill:       p.SkillName,
		dbctx:       p.DBCtx,
		user:        user,
		limiter:     newTokenBucket(p.Config.RatePerSec, p.Config.RateBurst),
		baseURL:     strings.TrimRight(p.Config.BaseURL, "/"),
		tokenHeader: hdr,
		token:       token,
		denyPaths:   mergeDenyPaths(p.Config.DenyPaths),
		client:      &http.Client{Timeout: 15 * time.Second},
		ln:          ln,
		path:        sockPath,
		connSem:     make(chan struct{}, maxGatewayConns),
	}
	go g.acceptLoop()
	return g, nil
}

// resolveUserToken returns a usable API token for user: an existing one if
// present, else a freshly minted + cache-injected one (persistent, identifiable).
func resolveUserToken(dbctx *ctx.Context, user *models.User, cache func(string, *models.User)) (string, error) {
	toks, err := models.GetTokensByUsername(dbctx, user.Username)
	if err != nil {
		return "", err
	}
	if len(toks) > 0 {
		return toks[0].Token, nil
	}
	tok, err := randToken()
	if err != nil {
		return "", err
	}
	if _, err := models.AddToken(dbctx, user.Username, tok, "skill-gateway"); err != nil {
		return "", err
	}
	if cache != nil {
		cache(tok, user)
	}
	logger.Infof("skill-gateway: created api token for %s (had none)", user.Username)
	return tok, nil
}

func randToken() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// SocketPath is the host UDS path to bind-mount into the sandbox.
func (g *Gateway) SocketPath() string { return g.path }

func (g *Gateway) Close() {
	if g == nil || !g.closed.CompareAndSwap(false, true) {
		return
	}
	if g.ln != nil {
		_ = g.ln.Close()
	}
	_ = os.Remove(g.path)
}

func (g *Gateway) acceptLoop() {
	for {
		c, err := g.ln.Accept()
		if err != nil {
			if g.closed.Load() {
				return
			}
			logger.Warningf("skill-gateway[%s]: accept: %v", g.execID, err)
			return
		}
		// Per-exec connection cap; blocking acquire backpressures the accept loop.
		// The per-request read+write deadlines in handleConn guarantee a slot
		// frees even against a peer that stalls mid-send or stops reading our
		// response, so a connection-flooding skill can't exhaust host goroutines/fds.
		g.connSem <- struct{}{}
		go func() {
			defer func() { <-g.connSem }()
			g.handleConn(c)
		}()
	}
}

// request / response are the newline-delimited JSON wire protocol. The skill
// opens the socket and exchanges one JSON object per line:
//
//	-> {"method":"GET","path":"/alert-rules","query":{"bgid":"1"}}
//	<- {"ok":true,"status":200,"data":{"dat":[...],"err":""}}
//
// GET is honored by default; POST is honored only for the read-only data-query
// allowlist, where the query goes in `body` (a JSON object), e.g.:
//
//	-> {"method":"POST","path":"/ds-query","body":{"cate":"prometheus","datasource_id":1,"query":[...]}}
//
// path is relative to n9e's /api/n9e prefix (the prefix is added by the gateway
// and tolerated if the skill includes it).
type request struct {
	Method string            `json:"method"`
	Path   string            `json:"path"`
	Query  map[string]string `json:"query"`
	Body   json.RawMessage   `json:"body,omitempty"` // JSON body for POST data-query endpoints
}

type response struct {
	OK     bool   `json:"ok"`
	Status int    `json:"status,omitempty"`
	Data   any    `json:"data,omitempty"`
	Error  string `json:"error,omitempty"`
}

func (g *Gateway) handleConn(c net.Conn) {
	defer c.Close()
	if err := verifyPeer(c); err != nil {
		logger.Warningf("skill-gateway[%s]: %v", g.execID, err)
		return
	}
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 0, 8*1024), 1<<20)
	w := bufio.NewWriter(c)
	for {
		// Refresh the idle read deadline before each request so a connection held
		// open without sending (slowloris) is reclaimed instead of parking this
		// goroutine indefinitely.
		_ = c.SetReadDeadline(time.Now().Add(gatewayIdleTimeout))
		if !sc.Scan() {
			return
		}
		resp := g.handleRequest(sc.Bytes())
		b, _ := json.Marshal(resp)
		// A peer that stops reading would otherwise block Flush on a large (≤1 MiB)
		// response indefinitely, pinning this goroutine and its conn-cap slot; the
		// write deadline reclaims it. Fresh window so a slow handleRequest doesn't
		// eat into it.
		_ = c.SetWriteDeadline(time.Now().Add(gatewayIdleTimeout))
		_, _ = w.Write(b)
		_ = w.WriteByte('\n')
		if w.Flush() != nil {
			return
		}
	}
}

func (g *Gateway) handleRequest(line []byte) response {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return response{Error: "invalid request json: " + err.Error()}
	}
	if !g.limiter.allow() {
		g.audit("-", req.Path, "rate_limited")
		return response{Error: "rate limit exceeded for this skill execution"}
	}
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}
	p := normalizePath(req.Path)
	if err := g.checkAllowed(method, p); err != nil {
		g.audit(method, p, "denied")
		return response{Error: err.Error()}
	}
	if len(req.Body) > maxReqBodyBytes {
		g.audit(method, p, "body_too_large")
		return response{Error: fmt.Sprintf("request body too large (%d > %d bytes)", len(req.Body), maxReqBodyBytes)}
	}
	return g.proxy(method, p, req.Query, req.Body)
}

// checkAllowed is the method+path gate: GET by default (deny-list), POST only for
// the read-only data-query allowlist, and a small allow-exception for safe
// sub-paths shadowed by a broad deny prefix. Everything else is refused.
func (g *Gateway) checkAllowed(method, p string) error {
	if !strings.HasPrefix(p, "/") {
		return fmt.Errorf("path must start with /")
	}
	// Reject percent-encoding outright: the deny-list matches the raw path, but
	// n9e's server decodes %xx before routing, so a path like "/%64atasources"
	// would dodge the "/datasource" prefix here yet hit the datasource endpoint
	// upstream — exfiltrating the secrets the deny-list exists to block. Path
	// segments never need encoding (query params go through the separate `query`
	// field, which url.Values encodes), so a "%" can only be an evasion attempt.
	if strings.Contains(p, "%") {
		return fmt.Errorf("path must not be percent-encoded; put parameters in the query field")
	}
	switch method {
	case http.MethodGet:
		// A safe sub-path re-permitted despite a broad deny prefix (e.g. the
		// secret-redacted /datasource/brief) bypasses the deny-list below.
		if getAllowExceptions[p] {
			return nil
		}
	case http.MethodPost:
		// POST is refused for everything except the read-only data-query allowlist.
		if !postAllowN9eAPIPaths[p] {
			return fmt.Errorf("POST %q is not allowed: the gateway permits POST only for read-only data-query endpoints", p)
		}
	default:
		return fmt.Errorf("method %s not allowed: the skill gateway allows GET, or POST to a read-only query endpoint", method)
	}
	// Deny-list applies to both GET and POST (defense in depth).
	low := strings.ToLower(p)
	for _, d := range g.denyPaths {
		if strings.HasPrefix(low, d) {
			return fmt.Errorf("path %q is blacklisted (sensitive data or non-read endpoint)", p)
		}
	}
	return nil
}

// proxy forwards the call to n9e's own API as the bound user and returns the
// (size-capped) response. n9e's route middleware enforces the user's RBAC.
func (g *Gateway) proxy(method, p string, query map[string]string, body []byte) response {
	u := g.baseURL + "/api/n9e" + p
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}
	httpReq, err := http.NewRequest(method, u, bodyReader)
	if err != nil {
		g.audit(method, p, "error")
		return response{Error: "build request: " + err.Error()}
	}
	if len(query) > 0 {
		q := url.Values{}
		for k, v := range query {
			q.Set(k, v)
		}
		httpReq.URL.RawQuery = q.Encode()
	}
	httpReq.Header.Set(g.tokenHeader, g.token)
	httpReq.Header.Set("X-Language", "en")
	if len(body) > 0 {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := g.client.Do(httpReq)
	if err != nil {
		g.audit(method, p, "error")
		return response{Error: "upstream call failed: " + err.Error()}
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes))
	g.audit(method, p, fmt.Sprintf("status_%d", resp.StatusCode))

	out := response{OK: resp.StatusCode >= 200 && resp.StatusCode < 300, Status: resp.StatusCode}
	var parsed any
	if json.Unmarshal(respBody, &parsed) == nil {
		out.Data = parsed
	} else {
		out.Data = string(respBody)
	}
	if !out.OK && out.Error == "" {
		out.Error = fmt.Sprintf("n9e api returned status %d", resp.StatusCode)
	}
	return out
}

// normalizePath cleans a skill-supplied path: strips query/fragment, ensures a
// leading slash, tolerates an included /api/n9e prefix, and collapses any "..".
func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = strings.TrimPrefix(p, "/api/n9e")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = path.Clean(p)
	return p
}

func (g *Gateway) audit(method, p, result string) {
	logger.Infof("skill-gateway[%s] skill=%s user=%s %s %s result=%s", g.execID, g.skill, g.user.Username, method, p, result)
	metricGatewayCalls.WithLabelValues(safeMethodLabel(method), result).Inc()
}

// safeMethodLabel bounds metric cardinality: the request method is skill-supplied
// and reaches audit() even on the deny path (a non-GET is rejected but still
// audited), so an unbounded method string would let a skill mint arbitrary label
// values. Collapse anything but the known verbs (and the rate-limited "-" marker)
// to "other".
func safeMethodLabel(method string) string {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch, "-":
		return method
	}
	return "other"
}
