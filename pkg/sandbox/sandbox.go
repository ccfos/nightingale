package sandbox

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"
)

// Sandbox is the control-plane singleton: it probes host capabilities once at
// New(), selects an Engine (with degradation), enforces host-level admission,
// and wraps every run with metrics + a structured audit record. Business code
// holds one Sandbox and calls Run(); the chosen engine is invisible to it.
type Sandbox struct {
	cfg        Config
	caps       Capabilities
	tier       Tier
	tierReason string

	engine Engine // nil when disabled
	reason string // why disabled (when engine == nil)
	// unsafeReason records why no stronger engine was available, set when the
	// selected engine is the unsafe-exec floor. Surfaced in the startup WARNING
	// so operators see what to install for real isolation.
	unsafeReason string
	adm          *admission
}

// New probes the host, selects the engine, and returns a ready controller. It
// never fails: an unusable host yields a disabled Sandbox whose Run() returns a
// *DisabledError with an actionable reason. cfg is PreCheck'd defensively.
func New(cfg Config) *Sandbox {
	cfg.PreCheck()
	caps := probeCapabilities()

	// Embedded assets (release builds, -tags sandbox_embed, §9.3): a self-
	// contained binary carries its own bwrap + python-base, extracted to the
	// data dir at startup. They take precedence over host-provided ones so
	// "install + run" needs no external bwrap or Rootfs.Path. Default builds
	// embed nothing, so this is a no-op.
	if bwrapPath, basePath, initPath, err := extractEmbeddedAssets(cfg.DataDir); err != nil {
		caps.note("embedded asset extraction failed: %v", err)
	} else {
		if bwrapPath != "" {
			caps.BwrapPath = bwrapPath
		}
		if basePath != "" && cfg.Rootfs.Path == "" {
			cfg.Rootfs.Path = basePath
			caps.note("using embedded python-base at %s", basePath)
		}
		if initPath != "" {
			caps.InitPath = initPath
			caps.note("using embedded egress forwarder at %s", initPath)
		}
	}

	s := &Sandbox{
		cfg:  cfg,
		caps: caps,
		adm:  newAdmission(cfg.Admission),
	}
	s.tier, s.tierReason = selectTier(cfg, s.caps)

	if cfg.Disabled {
		s.disable("sandbox.disabled=true")
		return s
	}
	s.resolveEngine()
	s.logStartup()
	return s
}

// resolveEngine walks a strength-ordered feasibility ladder and selects the
// first engine this host can actually build (fail-open + max-isolation ladder,
// see spec 2026-07-01-sandbox-fail-open). unsafe-exec is the universal floor so
// skill scripts stay runnable on any host; RequireIsolation=true refuses that
// floor (fail-closed) for security-sensitive deployments. It never lowers
// isolation silently — every degradation is recorded and logged loudly.
func (s *Sandbox) resolveEngine() {
	cfg := s.cfg

	// Candidate list, strongest first. auto walks the full ladder; an explicit
	// engine forces that one, then falls through to the unsafe-exec floor.
	var candidates []string
	if cfg.Engine == "" || strings.EqualFold(cfg.Engine, "auto") {
		candidates = strengthOrder
	} else {
		name, ok := configEngineToName(cfg.Engine)
		if !ok {
			s.disable(fmt.Sprintf("unknown sandbox.engine %q (want auto|bubblewrap|confined|unsafe)", cfg.Engine))
			return
		}
		candidates = []string{name}
		if name != EngineUnsafe {
			candidates = append(candidates, EngineUnsafe) // degradation floor
		}
	}

	var lastReason string
	for _, name := range candidates {
		if name == EngineUnsafe {
			// The no-isolation floor. RequireIsolation refuses it outright.
			if cfg.RequireIsolation {
				reason := "RequireIsolation=true and no isolation-capable engine is available on this host"
				if lastReason != "" {
					reason += " (" + lastReason + ")"
				}
				s.disable(reason)
				return
			}
			if f, ok := lookupEngine(EngineUnsafe); ok {
				if eng, err := f(cfg, s.caps); err == nil {
					s.engine = eng
					s.tier = tierForEngine(EngineUnsafe)
					s.unsafeReason = lastReason
					return
				} else {
					lastReason = fmt.Sprintf("unsafe-exec unavailable: %v", err)
				}
			} else {
				lastReason = "unsafe-exec engine not compiled into this build"
			}
			break
		}

		f, ok := lookupEngine(name)
		if !ok {
			// Not compiled in (e.g. runsc/nsjail in a default build). Skip; only
			// note it so an explicit request has an actionable reason.
			lastReason = fmt.Sprintf("engine %q not compiled into this build", name)
			continue
		}
		eng, err := f(cfg, s.caps)
		if err != nil {
			lastReason = fmt.Sprintf("engine %q not usable on this host: %v", name, err)
			logger.Warningf("sandbox: %s", lastReason)
			continue
		}
		s.engine = eng
		s.tier = tierForEngine(name)
		return
	}

	// Fell off the ladder with no engine (unsafe refused or unbuildable).
	if lastReason == "" {
		lastReason = s.tierReason
	}
	s.disable(lastReason)
}

func (s *Sandbox) disable(reason string) {
	s.engine = nil
	s.reason = reason
	s.tier = TierDisabled
}

func (s *Sandbox) logStartup() {
	for _, n := range s.caps.Notes {
		logger.Infof("sandbox probe: %s", n)
	}
	switch {
	case s.engine == nil:
		logger.Warningf("sandbox: SKILL EXECUTION DISABLED — %s (tier=%s, os=%s, kernel=%s)",
			s.reason, s.tier, s.caps.OS, s.caps.KernelVersion)
		return
	case s.engine.Name() == EngineUnsafe:
		reason := s.unsafeReason
		if reason == "" {
			reason = "no isolation-capable engine available on this host"
		}
		logger.Warningf("sandbox: SKILL EXECUTION RUNNING WITHOUT ISOLATION (unsafe-exec) — "+
			"install bubblewrap + a python-base rootfs for real isolation, or set "+
			"Sandbox.RequireIsolation=true to refuse. reason: %s (tier=%s, os=%s, kernel=%s)",
			reason, s.tier, s.caps.OS, s.caps.KernelVersion)
	default:
		logger.Infof("sandbox: ready engine=%s tier=%s os=%s kernel=%s caps=%+v",
			s.engine.Name(), s.tier, s.caps.OS, s.caps.KernelVersion, s.engine.Caps())
	}

	switch strings.ToLower(strings.TrimSpace(s.cfg.Egress)) {
	case EgressOff, EgressOpen, EgressAllowlist:
	default:
		logger.Warningf("sandbox: unrecognized Egress=%q (want off|open|allowlist) — treating as off (no skill network)", s.cfg.Egress)
	}
}

// Enabled reports whether skill execution can run on this host.
func (s *Sandbox) Enabled() bool { return s != nil && s.engine != nil }

// DisabledReason returns the actionable reason execution is off ("" when on).
func (s *Sandbox) DisabledReason() string {
	if s == nil {
		return "sandbox not initialized"
	}
	if s.engine != nil {
		return ""
	}
	return s.reason
}

// EngineName / Tier / Caps expose state for audit + admin display.
func (s *Sandbox) EngineName() string {
	if s == nil || s.engine == nil {
		return ""
	}
	return s.engine.Name()
}
func (s *Sandbox) Tier() Tier                 { return s.tier }
func (s *Sandbox) Capabilities() Capabilities { return s.caps }
func (s *Sandbox) Config() Config             { return s.cfg }

// EngineCaps reports what the selected backend can actually enforce on this host
// (zero value when disabled). The control plane consults it to decide whether to
// wire egress (needs Network) and the Skill Gateway (needs a mount namespace to
// bind the socket) for a run — both are bubblewrap-only in phase 1 (§10/§12).
func (s *Sandbox) EngineCaps() EngineCaps {
	if s == nil || s.engine == nil {
		return EngineCaps{}
	}
	return s.engine.Caps()
}

// Run executes spec under the selected engine, gated by admission control and
// wrapped with metrics. A *DisabledError means the host can't run skills; an
// *admissionError / ctx error means the run was refused/queued out. A script
// that runs and exits non-zero returns a nil error with ExecResult populated.
func (s *Sandbox) Run(ctx context.Context, spec ExecSpec) (ExecResult, error) {
	trigger := spec.TriggerType
	if trigger == "" {
		trigger = "unknown"
	}

	if !s.Enabled() {
		metricPolicyDeny.WithLabelValues("disabled").Inc()
		return ExecResult{}, &DisabledError{Reason: s.reason}
	}

	release, err := s.adm.acquire(ctx, spec.Resources.MemoryMB)
	if err != nil {
		reason := "admission"
		var ae *admissionError
		if errors.As(err, &ae) {
			reason = ae.reason
		} else if ctx.Err() != nil {
			reason = "queue_timeout"
		}
		metricPolicyDeny.WithLabelValues(reason).Inc()
		return ExecResult{}, err
	}
	defer release()

	metricExecActive.Inc()
	defer metricExecActive.Dec()

	engineName := s.engine.Name()
	start := time.Now()
	res, runErr := s.engine.Run(ctx, spec)
	res.Engine = engineName
	if res.Duration == 0 {
		res.Duration = time.Since(start)
	}

	status := "ok"
	if runErr != nil {
		status = "error"
		if res.Error == "" {
			res.Error = runErr.Error()
		}
	} else if res.ExitCode != 0 {
		status = "error"
	}
	metricExecTotal.WithLabelValues(engineName, status, trigger).Inc()
	metricExecDuration.WithLabelValues(engineName, trigger).Observe(res.Duration.Seconds())
	if res.KilledBy != "" {
		metricExecKilled.WithLabelValues(killReasonLabel(res.KilledBy), trigger).Inc()
	}
	return res, runErr
}

// killReasonLabel collapses "seccomp:<syscall>" to "seccomp" for a bounded
// metric label cardinality.
func killReasonLabel(killedBy string) string {
	if i := strings.IndexByte(killedBy, ':'); i >= 0 {
		return killedBy[:i]
	}
	return killedBy
}

// Preflight probes the host and returns a human-readable self-check report
// (design §17). Intended for a future `n9e sandbox preflight` subcommand and
// admin diagnostics: it tells the operator which tier/engine would be selected
// and, when execution would be disabled, what to enable.
func Preflight(cfg Config) string {
	s := New(cfg)
	var b strings.Builder
	b.WriteString("n9e sandbox preflight\n")
	fmt.Fprintf(&b, "  os/kernel        : %s %s\n", s.caps.OS, s.caps.KernelVersion)
	fmt.Fprintf(&b, "  unprivileged userns: %v\n", s.caps.UserNS)
	fmt.Fprintf(&b, "  cgroup v2 (deleg) : %v\n", s.caps.CgroupV2Delegated)
	fmt.Fprintf(&b, "  seccomp           : %v\n", s.caps.Seccomp)
	fmt.Fprintf(&b, "  landlock          : %v\n", s.caps.Landlock)
	fmt.Fprintf(&b, "  bwrap binary      : %s\n", strOrNone(s.caps.BwrapPath))
	fmt.Fprintf(&b, "  selected tier     : %s\n", s.tier)
	if s.Enabled() {
		fmt.Fprintf(&b, "  RESULT            : ENABLED via engine=%s\n", s.EngineName())
	} else {
		fmt.Fprintf(&b, "  RESULT            : DISABLED — %s\n", s.reason)
	}
	for _, n := range s.caps.Notes {
		fmt.Fprintf(&b, "  note: %s\n", n)
	}
	return b.String()
}

func strOrNone(s string) string {
	if s == "" {
		return "(not found)"
	}
	return s
}
