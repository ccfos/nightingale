package skillgateway

import (
	"bufio"
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
//   - Only GET is allowed — all writes (POST/PUT/DELETE/PATCH) are refused
//     wholesale, so there is no write surface to blacklist.
//   - A path blacklist (built-in defaults + Deny.N9eAPI) refuses the GETs that
//     return secrets (datasource configs, notify-channel secrets, user tokens,
//     SSO configs, datasource proxy). This is fail-OPEN by nature: a read endpoint
//     not on the list is reachable, so the default list must stay comprehensive.
//   - n9e's own route middleware still enforces the user's RBAC + busi-group
//     scope on every forwarded call (the gateway acts strictly AS the user).
//
// Identity is the chat session owner (bound at Start, never supplied by the
// script). The user's API token is resolved host-side and used for the loopback
// auth header — it is NEVER passed into the sandbox (§12.1).

// maxRespBytes caps a forwarded response so a skill can't pull an unbounded
// payload back through the gateway.
const maxRespBytes = 1 << 20 // 1 MiB

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

	ln     net.Listener
	path   string
	closed atomic.Bool
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
		go g.handleConn(c)
	}
}

// request / response are the newline-delimited JSON wire protocol. The skill
// opens the socket and exchanges one JSON object per line:
//
//	-> {"method":"GET","path":"/alert-rules","query":{"bgid":"1"}}
//	<- {"ok":true,"status":200,"data":{"dat":[...],"err":""}}
//
// Only GET is honored; path is relative to n9e's /api/n9e prefix (the prefix is
// added by the gateway and tolerated if the skill includes it).
type request struct {
	Method string            `json:"method"`
	Path   string            `json:"path"`
	Query  map[string]string `json:"query"`
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
	for sc.Scan() {
		resp := g.handleRequest(sc.Bytes())
		b, _ := json.Marshal(resp)
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
	return g.proxy(method, p, req.Query)
}

// checkAllowed is the deny-list gate: GET only, and the path must not match a
// blacklisted prefix.
func (g *Gateway) checkAllowed(method, p string) error {
	if method != http.MethodGet {
		return fmt.Errorf("method %s not allowed: the skill gateway is read-only (GET only)", method)
	}
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
func (g *Gateway) proxy(method, p string, query map[string]string) response {
	u := g.baseURL + "/api/n9e" + p
	httpReq, err := http.NewRequest(method, u, nil)
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

	resp, err := g.client.Do(httpReq)
	if err != nil {
		g.audit(method, p, "error")
		return response{Error: "upstream call failed: " + err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes))
	g.audit(method, p, fmt.Sprintf("status_%d", resp.StatusCode))

	out := response{OK: resp.StatusCode >= 200 && resp.StatusCode < 300, Status: resp.StatusCode}
	var parsed any
	if json.Unmarshal(body, &parsed) == nil {
		out.Data = parsed
	} else {
		out.Data = string(body)
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
