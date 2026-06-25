package skillgateway

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync/atomic"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/logger"
)

// Config is the gateway's slice of the global skill policy (§16.3). It is the
// same envelope the egress proxy reads from — grantable + deny — plus the
// per-exec rate limit.
type Config struct {
	GrantableAPI []string // skill_policy.grantable_n9e_api (allow envelope)
	DenyAPI      []string // deny.n9e_api (hard deny, e.g. *:write/*:delete/user:*)
	RatePerSec   float64  // per-exec request rate (default 5)
	RateBurst    int      // per-exec burst (default 10)
}

// Params binds one gateway instance to one execution. UserID is the chat session
// owner (ExecSpec.UserID) — the only identity the gateway will ever act as.
type Params struct {
	ExecID    string
	SkillName string
	UserID    int64
	DBCtx     *ctx.Context
	Config    Config
}

// Gateway is a running per-exec gateway. Its identity (user + busi-group scope)
// is resolved once at Start and is immutable for the life of the socket.
type Gateway struct {
	execID  string
	skill   string
	dbctx   *ctx.Context
	user    *models.User
	bgids   []int64
	isAdmin bool
	cfg     Config
	limiter *tokenBucket

	ln     net.Listener
	path   string
	closed atomic.Bool
}

// Start resolves the bound user, binds sockPath (host-private 0600), and begins
// serving. The socket is meant to be bind-mounted into exactly one sandbox at
// GatewaySocketTarget (§12.1). Caller must Close() it when the run ends.
func Start(sockPath string, p Params) (*Gateway, error) {
	if p.DBCtx == nil {
		return nil, errors.New("skill-gateway: nil db context")
	}
	if p.UserID <= 0 {
		return nil, errors.New("skill-gateway: no bound user id (identity must come from the chat session owner)")
	}
	user, err := models.UserGetById(p.DBCtx, p.UserID)
	if err != nil {
		return nil, fmt.Errorf("skill-gateway: resolve user %d: %w", p.UserID, err)
	}
	if user == nil {
		return nil, fmt.Errorf("skill-gateway: bound user %d not found", p.UserID)
	}
	isAdmin := user.IsAdmin()
	var bgids []int64
	if !isAdmin {
		if bgids, err = models.MyBusiGroupIds(p.DBCtx, user.Id); err != nil {
			return nil, fmt.Errorf("skill-gateway: load busi groups for user %d: %w", user.Id, err)
		}
	}

	_ = os.Remove(sockPath) // clear a stale socket from a crashed run
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("skill-gateway: listen %s: %w", sockPath, err)
	}
	_ = os.Chmod(sockPath, 0o600)

	g := &Gateway{
		execID:  p.ExecID,
		skill:   p.SkillName,
		dbctx:   p.DBCtx,
		user:    user,
		bgids:   bgids,
		isAdmin: isAdmin,
		cfg:     p.Config,
		limiter: newTokenBucket(p.Config.RatePerSec, p.Config.RateBurst),
		ln:      ln,
		path:    sockPath,
	}
	go g.acceptLoop()
	return g, nil
}

// SocketPath is the host UDS path to bind-mount into the sandbox.
func (g *Gateway) SocketPath() string { return g.path }

// Close stops serving and removes the socket.
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

// request / response are the newline-delimited JSON wire protocol. A skill (or a
// thin SDK helper) opens the socket and exchanges one JSON object per line:
//
//	-> {"op":"list_alert_rules","args":{}}
//	<- {"ok":true,"data":{"total":3,"items":[...]}}
type request struct {
	Op   string         `json:"op"`
	Args map[string]any `json:"args"`
}

type response struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

func (g *Gateway) handleConn(c net.Conn) {
	defer c.Close()
	if err := verifyPeer(c); err != nil {
		logger.Warningf("skill-gateway[%s]: %v", g.execID, err)
		return
	}
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 0, 8*1024), 1<<20) // cap one request line at 1 MiB
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
		g.audit(req.Op, "rate_limited")
		return response{Error: "rate limit exceeded for this skill execution"}
	}
	spec, ok := ops[req.Op]
	if !ok {
		g.audit(req.Op, "unknown_op")
		return response{Error: fmt.Sprintf("unknown operation %q", req.Op)}
	}
	if err := g.authorize(spec); err != nil {
		g.audit(req.Op, "denied")
		return response{Error: err.Error()}
	}
	data, err := spec.Handler(g, req.Args)
	if err != nil {
		g.audit(req.Op, "error")
		return response{Error: err.Error()}
	}
	g.audit(req.Op, "ok")
	return response{OK: true, Data: data}
}

// authorize enforces the three gates of §12.2, in order: the platform grantable
// envelope, the hard-deny list, then the bound user's own RBAC. Busi-group
// scoping happens later in the handler, so even a passing op returns ≤ what the
// user can see.
func (g *Gateway) authorize(spec opSpec) error {
	if spec.Grant == "" {
		return nil // identity-only op (whoami) — no data, always safe
	}
	if !grantMatched(g.cfg.GrantableAPI, spec.Grant) {
		return fmt.Errorf("not permitted: %q is not in grantable_n9e_api (ask an admin to allow it)", spec.Grant)
	}
	if grantMatched(g.cfg.DenyAPI, spec.Grant) {
		return fmt.Errorf("hard-denied by policy: %q", spec.Grant)
	}
	if spec.Operation != "" {
		has, err := g.user.CheckPerm(g.dbctx, spec.Operation)
		if err != nil {
			return fmt.Errorf("permission check failed: %w", err)
		}
		if !has {
			return fmt.Errorf("forbidden: user %q lacks operation %s", g.user.Username, spec.Operation)
		}
	}
	return nil
}

func (g *Gateway) audit(op, result string) {
	logger.Infof("skill-gateway[%s] skill=%s user=%s op=%s result=%s", g.execID, g.skill, g.user.Username, op, result)
	metricGatewayCalls.WithLabelValues(safeOpLabel(op), result).Inc()
}

// safeOpLabel bounds metric cardinality: only known ops appear as themselves,
// everything else collapses to "unknown".
func safeOpLabel(op string) string {
	if _, ok := ops[op]; ok {
		return op
	}
	return "unknown"
}
