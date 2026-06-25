// Package sandbox is the OS-agnostic control plane that runs untrusted Skill
// Python/Bash scripts under a pluggable isolation Engine. The control plane
// (policy, ExecSpec, admission, audit, capability probe, degradation) lives
// here and is the project's differentiated value; the isolation primitives
// (namespaces / seccomp / cgroup) are delegated to per-OS Engine backends and
// not reimplemented. Business code depends only on Sandbox.Run / the Engine
// interface and stays unaware of which backend executes a given spec.
//
// See ~/Desktop/n9e-sandbox-skill-execution-design-v2.md for the full design.
// This file declares the OS-neutral data model shared by every backend (§7.1).
package sandbox

import "time"

// NetworkPolicy is the egress posture for one execution (§10).
//   - none:   loopback-only / no outbound. The only mode wired in this phase.
//   - proxy:  egress allowed through the platform proxy + allowlist (future).
//   - direct: veth/NAT direct network, admin-only (future).
type NetworkPolicy string

const (
	NetworkNone   NetworkPolicy = "none"
	NetworkProxy  NetworkPolicy = "proxy"
	NetworkDirect NetworkPolicy = "direct"
)

// KilledBy enumerates the readable reason an execution was terminated. Empty
// means the process exited on its own. The Linux engines additionally emit
// "oom" / "pids" / "seccomp:<syscall>" once cgroup + seccomp are wired (§7.1).
const (
	KilledByTimeout = "timeout"
	KilledByOOM     = "oom"
	KilledByPids    = "pids"
)

// MountSpec is a single bind into the sandbox. Source is a host path; Target is
// the path as seen inside the sandbox. The unsafe engine cannot enforce these
// (no mount namespace) and treats them as advisory; the Linux engines bind them
// for real. Skill(ro) / input(ro) / workspace(rw) / output(rw) is the canonical
// set (§9.1).
type MountSpec struct {
	Source   string
	Target   string
	ReadOnly bool
}

// RootfsRef selects the immutable python-base plus any composable dependency
// layers to overlay (§9). On non-Linux / unsafe execution it is ignored and the
// host interpreter is used.
type RootfsRef struct {
	// Base is the resolved path of an extracted python-base@<hash>, or empty to
	// use the host interpreter (unsafe engine / dev).
	Base string
	// ExtraLayers are read-only dependency layers overlaid on top of Base
	// (deps/site-local and friends).
	ExtraLayers []string
}

// ResourceSpec is the per-execution resource envelope. Defaults come from the
// global policy (§14.1); the control plane clamps requested values to the
// skill ceilings before they reach an engine.
type ResourceSpec struct {
	Timeout   time.Duration // wall-clock; on expiry the whole cgroup is killed
	MemoryMB  int64         // cgroup memory.max (Linux) / advisory (unsafe)
	CPUQuota  string        // cgroup cpu.max, e.g. "100000 100000" (1 core)
	Pids      int64         // cgroup pids.max — fork-bomb guard
	FsizeMB   int64         // rlimit FSIZE
	Nofile    int64         // rlimit NOFILE
	StdoutMax int64         // stdout byte cap before truncation
	StderrMax int64         // stderr byte cap before truncation
}

// SecurityProfile names the seccomp profile and Landlock rules an engine should
// apply. Profile is a logical name ("python-minimal" / "bash-minimal") resolved
// per-backend; the unsafe engine ignores it.
type SecurityProfile struct {
	Profile         string   // seccomp profile name
	LandlockReadDir []string // additional read-only roots
	NoNewPrivs      bool
}

// ExecSpec is the OS-agnostic description of a single short-lived execution.
// Every backend translates it into its own form (bwrap argv / OCI spec / bare
// exec). Identity fields are bound by the caller before launch — the script
// never supplies them (§12.1). TenantID is reserved for n9e-plus; in ccfos the
// isolation unit is UserID + the user's BusiGroup scope (§12 note).
type ExecSpec struct {
	ExecID    string // unique per execution; names the workspace + audit row
	TenantID  string
	UserID    string
	SessionID string
	SkillName string

	Command []string          // argv, e.g. ["python3", "/skill/main.py", "--flag"]
	Cwd     string            // working dir inside the sandbox (workspace)
	Env     map[string]string // whitelisted injection; host env is NOT inherited
	Stdin   []byte

	Rootfs    RootfsRef
	Mounts    []MountSpec
	Resources ResourceSpec
	Network   NetworkPolicy
	Policy    SecurityProfile

	// TriggerType labels who initiated the run for audit/metrics
	// ("llm_tool" / "api" / "test"). Kept here so the engine-agnostic audit
	// record can carry it without a side channel.
	TriggerType string

	// Audit carries free-form labels copied verbatim into the audit record.
	Audit map[string]string
}

// Artifact is a file produced under output/ that the control plane surfaces back
// to the caller (§7.1). Phase 1 leaves this empty; reserved for richer skills.
type Artifact struct {
	Name string
	Path string
	Size int64
}

// ExecResult is the OS-agnostic outcome of one execution (§7.1).
type ExecResult struct {
	ExitCode        int
	Stdout          []byte
	Stderr          []byte
	StdoutTruncated bool
	StderrTruncated bool
	Timeout         bool
	KilledBy        string // "" | timeout | oom | pids | seccomp:<syscall>
	Duration        time.Duration
	Error           string
	Artifacts       []Artifact

	// Engine records which backend actually executed the spec, for audit.
	Engine string
}
