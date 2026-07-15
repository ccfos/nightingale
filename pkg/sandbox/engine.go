package sandbox

import "context"

// Engine names — the stable identifiers used in config, metrics labels, and
// audit rows. Keep them in sync with Config.Engine accepted values.
const (
	EngineUnsafe   = "unsafe-exec"
	EngineBwrap    = "bubblewrap"
	EngineConfined = "container-confined"
	EngineRunsc    = "runsc"  // future
	EngineNsjail   = "nsjail" // future
)

// EngineCaps reports what a backend can actually do on the current host. It
// lets the control plane reason about (and audit) the effective isolation,
// independent of which engine name was selected.
type EngineCaps struct {
	Namespaces bool // creates user/mount/pid/net namespaces
	Seccomp    bool
	Landlock   bool
	Cgroup     bool // can place the process in a dedicated cgroup with limits
	Network    bool // can enforce NetworkPolicy other than "none"
}

// Engine is the pluggable isolation backend. Each implementation translates the
// OS-agnostic ExecSpec into its own form and runs one short-lived task. The
// long-process variant (Start()/ProcessHandle for MCP stdio) is intentionally
// omitted this phase (§7.2).
type Engine interface {
	// Name returns a stable engine identifier (one of the Engine* constants).
	Name() string
	// Caps reports the effective capabilities of this backend on this host.
	Caps() EngineCaps
	// Run executes spec to completion. A non-nil error means the engine could
	// not run the spec at all (setup failure); a script that runs and exits
	// non-zero is a successful Run with ExecResult.ExitCode != 0.
	Run(ctx context.Context, spec ExecSpec) (ExecResult, error)
}

// Tier is the capability tier chosen by the probe (§6). It drives default
// engine selection under engine=auto.
type Tier int

const (
	// TierDisabled — no usable engine and RequireIsolation refused the unsafe
	// floor (or sandbox.enabled=false). Otherwise a no-isolation host reports
	// TierUnsafe, not TierDisabled (fail-open).
	TierDisabled Tier = iota
	// TierConfined — Linux container, userns unavailable but seccomp+Landlock+
	// rlimit are, and the operator declared container_as_boundary. Uses the
	// container-confined engine (§5.3 / 档 0.5).
	TierConfined
	// TierBubblewrap — Linux + userns + seccomp + cgroup v2. Production default,
	// uses bubblewrap (档 1).
	TierBubblewrap
	// TierStrong — controlled host requiring the strongest isolation (gVisor).
	// Reserved; not implemented this phase (档 2).
	TierStrong
	// TierUnsafe — no isolation engine was usable, so skill scripts run via the
	// unsafe-exec floor (bare exec, control-plane limits only). Reported honestly
	// for audit/admin so operators can see when a host is running without
	// isolation. Appended last to keep the existing iota values stable.
	TierUnsafe
)

func (t Tier) String() string {
	switch t {
	case TierConfined:
		return "confined"
	case TierBubblewrap:
		return "bubblewrap"
	case TierStrong:
		return "strong"
	case TierUnsafe:
		return "unsafe"
	default:
		return "disabled"
	}
}

// strengthOrder lists the engines from strongest to weakest isolation. auto
// (Config.Engine=="" | "auto") walks it and selects the first engine this host
// can actually build; unsafe-exec is the universal floor (see resolveEngine).
var strengthOrder = []string{EngineRunsc, EngineBwrap, EngineConfined, EngineUnsafe}

// tierForEngine maps a selected engine name to the tier reported for audit and
// admin display, so Tier() reflects the ACTUAL isolation level in force rather
// than the host's ideal tier.
func tierForEngine(name string) Tier {
	switch name {
	case EngineRunsc:
		return TierStrong
	case EngineBwrap:
		return TierBubblewrap
	case EngineConfined:
		return TierConfined
	case EngineUnsafe:
		return TierUnsafe
	default:
		return TierDisabled
	}
}
