package sandbox

import (
	"strings"
	"time"
)

// Egress preset values for Config.Egress — the one-knob egress posture (§10.1).
const (
	EgressOff       = "off"       // no network (loopback-only netns)
	EgressOpen      = "open"      // public + private (RFC1918) via the audited proxy
	EgressAllowlist = "allowlist" // only SkillPolicy.EgressAllowlist hosts; private blocked
)

// N9eAPI preset values for Config.N9eAPI — the one-knob Skill Gateway posture
// (§12.1), parallel to Egress. Read-only either way; the Deny.N9eAPI floor
// (writes/deletes/user:*) applies on top, so a write op can never slip through.
const (
	N9eAPIReadonly = "readonly" // (default) skills may call read-only n9e ops as the launching user
	N9eAPIOff      = "off"      // no n9e API access (the gateway is not started)
)

// Config mirrors the `[Sandbox]` section (design §16.3). It is loaded as part of
// the top-level n9e config and PreCheck() fills defaults. SkillPolicy / Deny /
// EgressProxy drive the live egress proxy (§10) and Skill Gateway (§12).
type Config struct {
	Enabled bool

	// Engine selects the backend: auto | bubblewrap | confined | unsafe.
	// auto follows the capability tier table (§6).
	Engine string

	// Egress is the single-knob egress preset (§10.1): off | open | allowlist.
	//   - off:       no network (most isolated).
	//   - open:      (default) skills reach public AND private (RFC1918/ULA) hosts
	//                through the audited proxy, out of the box. The catastrophic
	//                floor stays blocked regardless: host loopback (127/8, ::1) and
	//                cloud-metadata / link-local (169.254/16, fe80::/10) are NEVER
	//                reachable — a skill must not hit n9e's own API/DB or steal
	//                cloud credentials. UDP blocked; every call audited.
	//   - allowlist: reach only SkillPolicy.EgressAllowlist hosts; private blocked.
	// It supersedes the lower-level DefaultPolicy.Network knob.
	Egress string

	// N9eAPI is the single-knob Skill Gateway preset (§12.1), parallel to Egress:
	//   - readonly: (default) a skill may call n9e's OWN /api/n9e/* via the Skill
	//               Gateway, GET-only, AS the launching user. The user's API token
	//               is held host-side and never given to the sandbox; n9e's route
	//               middleware still enforces that user's RBAC + busi-group scope.
	//               A path deny-list (built-in defaults + Deny.N9eAPI) refuses the
	//               secret-bearing reads (datasource configs, notify-channel
	//               secrets, user tokens, SSO, datasource proxy).
	//   - off:      no n9e API access (the gateway is not started).
	// Writes (POST/PUT/DELETE) are refused wholesale regardless of preset.
	// Unrecognized values fail safe to off.
	N9eAPI string

	// DevMode permits the unsafe engine when capabilities are insufficient
	// (non-Linux / userns off). Production must leave this false.
	DevMode bool

	// ContainerAsBoundary lets auto degrade to the container-confined engine
	// when userns is unavailable — an explicit operator acknowledgement that the
	// outer container is the host boundary (§5.3 / 档 0.5). Without it, auto
	// stays disabled rather than silently lowering isolation.
	ContainerAsBoundary bool

	DataDir string

	Rootfs        RootfsConfig
	DefaultPolicy PolicyConfig
	Admission     AdmissionConfig
	Skill         SkillLimits
	SkillPolicy   SkillPolicyConfig
	Deny          DenyConfig
	EgressProxy   EgressProxyConfig

	// SeccompMode is audit | enforce. First release runs audit (SECCOMP_RET_LOG)
	// to collect real denied syscalls before switching to enforce (§15).
	SeccompMode string
}

// RootfsConfig controls the python-base rootfs and overlay layers (§9).
type RootfsConfig struct {
	// Source: embedded (default, base baked into the binary) | path (external /
	// self-built base) | download (optional prebuilt extra layers only).
	Source string
	// Path overrides the embedded base when set (long-tail arch / self-patched).
	Path string
	// ExtraLayers are read-only dependency layers overlaid on top of base
	// (e.g. deps/site-local).
	ExtraLayers []string
}

// PolicyConfig is the default per-execution policy envelope (§8.2).
type PolicyConfig struct {
	Network        string
	TimeoutSeconds int
	MemoryMB       int64
	CPUQuota       string
	Pids           int64
	StdoutBytes    int64
	StderrBytes    int64
}

// AdmissionConfig is the host-level concurrency / memory budget (§14.2).
type AdmissionConfig struct {
	MaxConcurrent    int
	MaxTotalMemoryMB int64
}

// SkillLimits caps what a single skill execution may request (§16.3).
type SkillLimits struct {
	Enabled             bool
	MaxUploadBytes      int64
	MaxFiles            int
	AllowedNetworkModes []string
	MaxTimeoutSeconds   int
	MaxMemoryMB         int64
	MaxPids             int64
}

// SkillPolicyConfig is the global envelope shared by all skills (§11.2 / §16.3).
type SkillPolicyConfig struct {
	EgressAllowlist []string
	// GrantableN9eAPI is RESERVED (the Skill Gateway is now an HTTP passthrough
	// gated by a path deny-list, not grant tokens). Kept for config compatibility.
	GrantableN9eAPI []string
	JitConfirm      []string
}

// DenyConfig is the hard deny no skill/layer can override (§16.3).
type DenyConfig struct {
	EgressCIDRs []string
	// N9eAPI is the Skill Gateway deny-list: case-insensitive path PREFIXES (under
	// /api/n9e) refused on top of the gateway's built-in defaults — e.g.
	// ["/datasource", "/notify-channel"]. Adds to, never removes from, the floor.
	N9eAPI []string
}

// EgressProxyConfig configures the host-side egress proxy (§10). Phase 1 wires
// the simple form: per-exec UNIX-socket CONNECT/forward proxy with allowlist +
// DNS-pin + SSRF, no TLS interception (§10.3). The allowlist + hard-deny CIDRs
// live in SkillPolicy.EgressAllowlist / Deny.EgressCIDRs; this struct holds the
// transport knobs.
type EgressProxyConfig struct {
	// TLSInspect (MITM) is NOT phase 1 (§10.5); kept for forward-compat. When
	// true the proxy still tunnels without decrypting and logs a warning.
	TLSInspect bool

	// AllowPlainHTTP forwards absolute-form plain-HTTP in addition to HTTPS
	// CONNECT. Default true so http:// APIs work; set false for HTTPS-only egress.
	AllowPlainHTTP bool
	// DenyPrivateCIDRs is SUPERSEDED by the Egress preset (open allows private,
	// allowlist denies it) and no longer consulted. Kept for config compatibility.
	// loopback / link-local / metadata are always blocked regardless (ipDenied).
	DenyPrivateCIDRs bool
	// DenyUDP is informational: a CONNECT/forward proxy is TCP-only by
	// construction, so UDP/QUIC egress is already impossible (§10.4). Kept for
	// the admin-facing posture report.
	DenyUDP bool

	DialTimeoutSecs  int   // upstream connect timeout (default 10)
	IdleTimeoutSecs  int   // tunnel idle timeout (default 120)
	MaxResponseBytes int64 // reserved; unenforceable on an undecrypted CONNECT tunnel

	// ForwarderPath points at an external n9e-sandbox-init forwarder binary for
	// non-embedded builds. The embedded binary (-tags sandbox_embed) takes
	// precedence; this is the fallback so a plain `go build` can still run proxy
	// mode if the operator supplies the helper (parallel to Rootfs.Path).
	ForwarderPath string
}

// Defaults (mirrors §8.2 / §16.3).
const (
	defaultDataDir          = "/var/lib/n9e/sandbox"
	defaultTimeoutSeconds   = 30
	defaultMemoryMB         = 256
	defaultCPUQuota         = "100000 100000"
	defaultPids             = 32
	defaultStdoutBytes      = 262144
	defaultStderrBytes      = 262144
	defaultMaxConcurrent    = 32
	defaultMaxTotalMemoryMB = 8192
	defaultSkillMaxTimeout  = 300
	defaultSkillMaxMemoryMB = 1024
	defaultSkillMaxPids     = 128
	defaultSkillMaxUpload   = 1048576
	defaultSkillMaxFiles    = 100
)

// PreCheck fills zero-value defaults. It is idempotent and safe to call once at
// startup after config load. A non-zero value the operator set is preserved.
func (c *Config) PreCheck() {
	if c.Engine == "" {
		c.Engine = "auto"
	}
	if c.Egress == "" {
		c.Egress = EgressOpen
	}
	if c.N9eAPI == "" {
		c.N9eAPI = N9eAPIReadonly
	}
	if c.DataDir == "" {
		c.DataDir = defaultDataDir
	}
	if c.SeccompMode == "" {
		c.SeccompMode = "enforce"
	}
	if c.Rootfs.Source == "" {
		c.Rootfs.Source = "embedded"
	}

	p := &c.DefaultPolicy
	if p.Network == "" {
		p.Network = string(NetworkNone)
	}
	if p.TimeoutSeconds == 0 {
		p.TimeoutSeconds = defaultTimeoutSeconds
	}
	if p.MemoryMB == 0 {
		p.MemoryMB = defaultMemoryMB
	}
	if p.CPUQuota == "" {
		p.CPUQuota = defaultCPUQuota
	}
	if p.Pids == 0 {
		p.Pids = defaultPids
	}
	if p.StdoutBytes == 0 {
		p.StdoutBytes = defaultStdoutBytes
	}
	if p.StderrBytes == 0 {
		p.StderrBytes = defaultStderrBytes
	}

	a := &c.Admission
	if a.MaxConcurrent == 0 {
		a.MaxConcurrent = defaultMaxConcurrent
	}
	if a.MaxTotalMemoryMB == 0 {
		a.MaxTotalMemoryMB = defaultMaxTotalMemoryMB
	}

	s := &c.Skill
	if len(s.AllowedNetworkModes) == 0 {
		s.AllowedNetworkModes = []string{string(NetworkNone), string(NetworkProxy)}
	}
	if s.MaxTimeoutSeconds == 0 {
		s.MaxTimeoutSeconds = defaultSkillMaxTimeout
	}
	if s.MaxMemoryMB == 0 {
		s.MaxMemoryMB = defaultSkillMaxMemoryMB
	}
	if s.MaxPids == 0 {
		s.MaxPids = defaultSkillMaxPids
	}
	if s.MaxUploadBytes == 0 {
		s.MaxUploadBytes = defaultSkillMaxUpload
	}
	if s.MaxFiles == 0 {
		s.MaxFiles = defaultSkillMaxFiles
	}
}

// EgressPlan resolves the Egress preset into the effective proxy posture: whether
// to run the proxy at all, the host allowlist (["*"] = all hosts, used by open),
// and whether to deny RFC1918/ULA private IPs. The catastrophic floor (loopback +
// link-local/metadata) is always blocked in ipDenied regardless of this plan, so
// even "open" cannot reach the host's own services or the cloud metadata endpoint.
// An empty/unset value defaults to open (out-of-the-box network); a recognized-
// but-not-open value behaves as written; an unrecognized value falls back to off
// (fail safe — a typo must not silently turn network on).
func (c Config) EgressPlan() (proxy bool, allowlist []string, denyPrivate bool) {
	mode := strings.ToLower(strings.TrimSpace(c.Egress))
	if mode == "" {
		mode = EgressOpen // unset → default open
	}
	switch mode {
	case EgressOpen:
		return true, []string{"*"}, false
	case EgressAllowlist:
		return true, c.SkillPolicy.EgressAllowlist, true
	default: // off, or an unrecognized value → no network
		return false, nil, false
	}
}

// N9eAPIEnabled reports whether the Skill Gateway's read passthrough is on.
// readonly (the default, or unset) = on; off — or any unrecognized value,
// fail-safe — = the gateway is not started. The deny-list (Deny.N9eAPI + the
// gateway's built-in defaults) and GET-only gate bound what a skill can reach.
func (c Config) N9eAPIEnabled() bool {
	m := strings.ToLower(strings.TrimSpace(c.N9eAPI))
	return m == "" || m == N9eAPIReadonly
}

// DefaultResources builds a ResourceSpec from the configured default policy.
func (c *Config) DefaultResources() ResourceSpec {
	return ResourceSpec{
		Timeout:   time.Duration(c.DefaultPolicy.TimeoutSeconds) * time.Second,
		MemoryMB:  c.DefaultPolicy.MemoryMB,
		CPUQuota:  c.DefaultPolicy.CPUQuota,
		Pids:      c.DefaultPolicy.Pids,
		StdoutMax: c.DefaultPolicy.StdoutBytes,
		StderrMax: c.DefaultPolicy.StderrBytes,
	}
}
