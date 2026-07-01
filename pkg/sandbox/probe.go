package sandbox

import (
	"fmt"
	"runtime"
	"strings"
)

// Capabilities is the OS/kernel feature inventory gathered at startup (§6). The
// per-OS probe* files fill it; selectTier (here, OS-neutral) maps it to a Tier.
type Capabilities struct {
	OS                string
	KernelVersion     string
	UserNS            bool   // unprivileged user namespace creation
	CgroupV2Delegated bool   // cgroup v2 present and delegable
	Seccomp           bool   // seccomp-bpf available
	Landlock          bool   // Landlock LSM available (5.13+)
	BwrapPath         string // resolved bubblewrap binary path, or ""
	InitPath          string // resolved n9e-sandbox-init forwarder path, or "" (§10.2)
	Notes             []string
}

// note appends a human-readable observation used in the startup log + admin UI.
func (c *Capabilities) note(format string, a ...interface{}) {
	c.Notes = append(c.Notes, fmt.Sprintf(format, a...))
}

// selectTier maps probed capabilities + operator config to a capability tier
// (§6). It is deliberately OS-agnostic: probe_linux / probe_other decide the
// raw capability bits, this decides the policy. The returned reason explains
// the choice (and, when disabled, what to enable).
func selectTier(cfg Config, caps Capabilities) (Tier, string) {
	if caps.OS != "linux" {
		return TierDisabled, fmt.Sprintf("kernel-isolation sandbox is Linux-only; host OS is %q", caps.OS)
	}
	if caps.UserNS && caps.Seccomp && caps.CgroupV2Delegated {
		return TierBubblewrap, "unprivileged userns + seccomp + cgroup v2 available"
	}
	// userns unavailable but the cheap, zero-privilege layers are — degrade to
	// container-confined only if the operator explicitly accepts the outer
	// container as the boundary (no silent isolation downgrade, §5.3).
	if caps.Seccomp && caps.Landlock {
		if cfg.ContainerAsBoundary {
			return TierConfined, "userns unavailable; container_as_boundary set → container-confined"
		}
		return TierDisabled, "userns unavailable; set container_as_boundary=true to allow the container-confined engine, or enable unprivileged userns for bubblewrap"
	}
	return TierDisabled, "insufficient capabilities (need userns+seccomp+cgroup for bubblewrap, or seccomp+landlock+container_as_boundary for confined)"
}

// tierEngineName returns the auto-selected engine name for a tier, or "" when
// no production engine fits the tier (caller decides dev fallback).
func tierEngineName(t Tier) string {
	switch t {
	case TierBubblewrap:
		return EngineBwrap
	case TierConfined:
		return EngineConfined
	case TierStrong:
		return EngineRunsc
	default:
		return ""
	}
}

// baseCaps seeds the OS field so probe_* only fill the kernel bits.
func baseCaps() Capabilities {
	return Capabilities{OS: runtime.GOOS}
}

// configEngineToName resolves the short config name (auto handled by caller) to
// an internal engine name. Returns ("", false) for unknown values.
func configEngineToName(s string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "unsafe":
		return EngineUnsafe, true
	case "bubblewrap", "bwrap":
		return EngineBwrap, true
	case "confined", "container-confined":
		return EngineConfined, true
	case "runsc", "gvisor":
		return EngineRunsc, true
	case "nsjail":
		return EngineNsjail, true
	}
	return "", false
}
