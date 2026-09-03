package skillruntime

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	skillgateway "github.com/ccfos/nightingale/v6/aiagent/skill_gateway"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/sandbox"
	"github.com/toolkits/pkg/logger"
)

// control.go owns the per-execution control channels (§10 egress + §12 gateway):
// the out-of-sandbox brokers a run may need, started just before Run and torn
// down right after. Both are reached over UNIX sockets bind-mounted into the
// sandbox — no real network — so the script talks to them while staying isolated.
//
// Egress (§10) needs a real network namespace + the forwarder, so it is wired
// ONLY for engines that can enforce it (bubblewrap). The Skill Gateway (§12) is
// reachable on ANY engine: namespace engines bind its socket to a fixed in-
// sandbox path, while the unsafe-exec engine (which runs directly on the host) is
// handed the real host socket path via the same env var. So an unsafe-exec run
// still gets read-only n9e-API access — the gateway enforces RBAC + deny-paths
// server-side either way — just no egress proxy.

type controlChannels struct {
	egress  *sandbox.EgressProxy
	gateway *skillgateway.Gateway
	dir     string // per-exec host dir holding the sockets
	// bindMounts is true when the engine isolates via namespaces, so the sockets
	// are bind-mounted to fixed in-sandbox paths. When false (unsafe-exec), the
	// run reaches the sockets at their real host paths instead.
	bindMounts bool
}

// resolveNetwork picks a run's egress posture from the Egress preset (§10.1):
// proxy when Egress is open/allowlist AND the active engine can enforce it
// (bubblewrap has the forwarder); otherwise none.
func resolveNetwork(s *sandbox.Sandbox) sandbox.NetworkPolicy {
	proxy, _, _ := s.Config().EgressPlan()
	if !proxy {
		return sandbox.NetworkNone
	}
	if !s.EngineCaps().Network { // engine cannot enforce a proxy posture (no forwarder)
		return sandbox.NetworkNone
	}
	return sandbox.NetworkProxy
}

// setupControlChannels starts whichever channels this run needs. It is
// all-or-nothing: any failure closes what it started and returns an error so the
// caller can degrade to a no-network, no-gateway run (the safe fallback) rather
// than launching with a half-wired proxy. Returns (nil, nil) when nothing is
// needed.
func setupControlChannels(d Deps, execID, skillName string, netMode sandbox.NetworkPolicy, user *models.User) (*controlChannels, error) {
	cfg := d.Sandbox.Config()
	bindsMounts := d.Sandbox.EngineCaps().Namespaces

	needEgress := netMode == sandbox.NetworkProxy
	// The gateway works on any engine — namespace engines get a bind-mounted
	// socket, unsafe-exec gets the host socket path (see env()/mounts()). So it is
	// NOT gated on bindsMounts.
	needGateway := d.DBCtx != nil && user != nil && cfg.N9eAPIEnabled() && d.N9eBaseURL != ""
	if !needEgress && !needGateway {
		return nil, nil
	}

	dir := filepath.Join(cfg.DataDir, "run", execID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create control dir: %w", err)
	}
	cc := &controlChannels{dir: dir, bindMounts: bindsMounts}

	if needEgress {
		// open → allowlist=["*"], denyPrivate=false (public + private reachable);
		// allowlist → the configured hosts, denyPrivate=true. The loopback +
		// metadata floor is enforced unconditionally in the proxy regardless (§10.4).
		_, allowlist, denyPrivate := cfg.EgressPlan()
		ep, err := sandbox.StartEgressProxy(filepath.Join(dir, "egress.sock"), sandbox.EgressOptions{
			ExecID:         execID,
			Allowlist:      allowlist,
			DenyCIDRs:      cfg.Deny.EgressCIDRs,
			DenyPrivate:    denyPrivate,
			AllowPlainHTTP: !cfg.EgressProxy.DenyPlainHTTP,
			DialTimeout:    time.Duration(cfg.EgressProxy.DialTimeoutSecs) * time.Second,
			IdleTimeout:    time.Duration(cfg.EgressProxy.IdleTimeoutSecs) * time.Second,
			// OnNewDomain nil → deny non-allowlisted hosts (managed lockdown). A
			// future JIT confirmation hook plugs in here (§10.4/§11.2).
			OnAudit: egressAuditLogger(skillName),
		})
		if err != nil {
			cc.close()
			return nil, fmt.Errorf("start egress proxy: %w", err)
		}
		cc.egress = ep
	}

	if needGateway {
		gw, err := skillgateway.Start(filepath.Join(dir, "gw.sock"), skillgateway.Params{
			ExecID:     execID,
			SkillName:  skillName,
			UserID:     user.Id,
			DBCtx:      d.DBCtx,
			CacheToken: d.CacheToken,
			Config: skillgateway.Config{
				BaseURL:    d.N9eBaseURL,
				DenyPaths:  cfg.Deny.N9eAPI,
				RatePerSec: 5,
				RateBurst:  10,
			},
		})
		if err != nil {
			cc.close()
			return nil, fmt.Errorf("start skill gateway: %w", err)
		}
		cc.gateway = gw
	}
	return cc, nil
}

// mounts returns the control-socket bind-mounts for the ExecSpec (engine binds
// them onto the sandbox's private /run tmpfs).
func (c *controlChannels) mounts() []sandbox.MountSpec {
	if c == nil || !c.bindMounts {
		// Non-namespace engine (unsafe-exec): nothing is bind-mounted; the run
		// reaches the sockets at their real host paths (see env()).
		return nil
	}
	var m []sandbox.MountSpec
	if c.egress != nil {
		m = append(m, sandbox.MountSpec{Source: c.egress.SocketPath(), Target: sandbox.EgressSocketTarget})
	}
	if c.gateway != nil {
		m = append(m, sandbox.MountSpec{Source: c.gateway.SocketPath(), Target: sandbox.GatewaySocketTarget})
	}
	return m
}

// env returns the env injected for the channels: HTTP(S)_PROXY pointing at the
// in-netns forwarder, and the gateway socket path. Stock HTTP clients pick up the
// proxy with no code changes (§10.2).
func (c *controlChannels) env() map[string]string {
	if c == nil {
		return nil
	}
	e := map[string]string{}
	if c.egress != nil {
		proxyURL := "http://" + sandbox.EgressForwarderListen
		e["HTTP_PROXY"] = proxyURL
		e["HTTPS_PROXY"] = proxyURL
		e["http_proxy"] = proxyURL
		e["https_proxy"] = proxyURL
	}
	if c.gateway != nil {
		e[sandbox.GatewaySocketEnv] = gatewayEnvValue(c.bindMounts, c.gateway.SocketPath())
	}
	return e
}

// gatewayEnvValue is the value of N9E_SKILL_GATEWAY handed to the run: the fixed
// in-sandbox bind path for namespace engines, or the real host socket path for
// unsafe-exec (which has no mount namespace and runs directly on the host).
func gatewayEnvValue(bindMounts bool, hostSocketPath string) string {
	if bindMounts {
		return sandbox.GatewaySocketTarget
	}
	return hostSocketPath
}

func (c *controlChannels) close() {
	if c == nil {
		return
	}
	if c.egress != nil {
		c.egress.Close()
	}
	if c.gateway != nil {
		c.gateway.Close()
	}
	if c.dir != "" {
		_ = os.RemoveAll(c.dir)
	}
}

func egressAuditLogger(skill string) func(sandbox.EgressAudit) {
	return func(a sandbox.EgressAudit) {
		if a.Allowed {
			logger.Infof("sandbox egress[%s] skill=%s ALLOW %s %s:%s ip=%s up=%d down=%d dur=%s",
				a.ExecID, skill, a.Method, a.Host, a.Port, a.PinnedIP, a.BytesUp, a.BytesDown, a.Duration)
		} else {
			logger.Infof("sandbox egress[%s] skill=%s DENY %s %s:%s — %s",
				a.ExecID, skill, a.Method, a.Host, a.Port, a.Reason)
		}
	}
}
