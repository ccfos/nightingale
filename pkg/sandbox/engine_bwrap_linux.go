//go:build linux

package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/toolkits/pkg/logger"
)

// bwrapEngine is the production default (档 1, §5.1). It delegates the hard
// isolation primitives to the external `bwrap` binary — the same approach as
// Anthropic Claude Code's Linux backend — rather than reimplementing
// namespaces/userns/mounts in Go. bwrap builds a full user+mount+pid+ipc+uts
// +cgroup namespace set (and, for network=none, a loopback-only net namespace),
// drops capabilities, and applies no_new_privs by default. The control plane
// layers cgroup v2 resource limits on top (setupCgroup).
//
// VERIFICATION STATUS: this engine cross-compiles and registers, but has been
// written without a Linux test host. It requires integration testing on Linux
// (design §18.2). Outstanding hardening (TODO, design §8.1/§9):
//   - custom seccomp profile via bwrap --seccomp <fd> (currently unset; the
//     namespace+userns+cgroup boundary still applies);
//   - Landlock file rules;
//   - embedded python-base rootfs (this phase requires Rootfs.Path to point at
//     an external base; embedding is a follow-up).
type bwrapEngine struct {
	bwrap string
	base  string
	cfg   Config
}

func init() {
	registerEngine(EngineBwrap, func(cfg Config, caps Capabilities) (Engine, error) {
		if caps.BwrapPath == "" {
			return nil, fmt.Errorf("bwrap binary not found in PATH")
		}
		base := resolveRootfsBase(cfg)
		if base == "" {
			return nil, fmt.Errorf("no python-base rootfs available: set sandbox Rootfs.Path to an external base (embedded base is a TODO)")
		}
		return &bwrapEngine{bwrap: caps.BwrapPath, base: base, cfg: cfg}, nil
	})
}

func (e *bwrapEngine) Name() string { return EngineBwrap }

func (e *bwrapEngine) Caps() EngineCaps {
	return EngineCaps{Namespaces: true, Cgroup: true, Network: true}
}

func (e *bwrapEngine) Run(ctx context.Context, spec ExecSpec) (ExecResult, error) {
	if len(spec.Command) == 0 {
		return ExecResult{}, fmt.Errorf("empty command")
	}
	cg := setupCgroup(spec.ExecID, spec.Resources)
	defer cg.cleanup()

	args := e.buildArgs(spec)
	logger.Debugf("sandbox[bwrap]: skill=%s argv=%v", spec.SkillName, spec.Command)

	cmd := exec.Command(e.bwrap, args...)
	if len(spec.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(spec.Stdin)
	}
	out := &cappedBuffer{max: spec.Resources.StdoutMax}
	errb := &cappedBuffer{max: spec.Resources.StderrMax}
	cmd.Stdout = out
	cmd.Stderr = errb

	timeout := spec.Resources.Timeout
	if timeout <= 0 {
		timeout = time.Duration(defaultTimeoutSeconds) * time.Second
	}

	start := time.Now()
	runErr, timedOut := runLinuxCmd(cmd, timeout, cg)
	dur := time.Since(start)

	res := ExecResult{
		Stdout:          out.buf.Bytes(),
		Stderr:          errb.buf.Bytes(),
		StdoutTruncated: out.truncated,
		StderrTruncated: errb.truncated,
		Duration:        dur,
	}
	if timedOut {
		res.Timeout = true
		res.KilledBy = KilledByTimeout
		res.ExitCode = -1
		res.Error = fmt.Sprintf("killed after %s timeout", timeout)
		return res, nil
	}
	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}
	if reason := cgroupKilledReason(cg); reason != "" {
		res.KilledBy = reason
	}
	if runErr != nil {
		if _, ok := runErr.(*exec.ExitError); !ok {
			return res, fmt.Errorf("bwrap exec: %w", runErr)
		}
	}
	return res, nil
}

// buildArgs translates the ExecSpec into a bwrap command line. Command/Cwd/Env
// path values are remapped from their host locations to the bind targets.
//
// BASE CONTRACT (verified on Linux 2026-06-25): because the rootfs is bound
// read-only at "/", bwrap cannot mkdir the bind targets (/skill /input
// /workspace /output) under it — it fails with "Can't mkdir /skill: Read-only
// file system". The python-base rootfs MUST therefore ship those mount-point
// directories as empty dirs. The embedded-base build (§9.3, TODO) must create
// them; an externally-supplied Rootfs.Path must contain them too.
func (e *bwrapEngine) buildArgs(spec ExecSpec) []string {
	a := []string{
		"--unshare-user", "--unshare-pid", "--unshare-ipc", "--unshare-uts", "--unshare-cgroup",
		"--die-with-parent", "--new-session",
		"--clearenv",
		"--ro-bind", e.base, "/",
		"--proc", "/proc",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
	}
	if spec.Network == NetworkNone || spec.Network == "" {
		a = append(a, "--unshare-net")
	}
	for _, m := range spec.Mounts {
		if m.Source == "" || m.Target == "" {
			continue
		}
		if m.ReadOnly {
			a = append(a, "--ro-bind", m.Source, m.Target)
		} else {
			a = append(a, "--bind", m.Source, m.Target)
		}
	}
	for k, v := range spec.Env {
		a = append(a, "--setenv", k, remapPath(spec.Mounts, v))
	}
	if cwd := remapPath(spec.Mounts, spec.Cwd); cwd != "" {
		a = append(a, "--chdir", cwd)
	}
	a = append(a, "--")
	for _, c := range spec.Command {
		a = append(a, remapPath(spec.Mounts, c))
	}
	return a
}

// resolveRootfsBase returns the python-base path. This phase only supports an
// external base via Rootfs.Path; the embedded go:embed base (§9.3) is a TODO.
func resolveRootfsBase(cfg Config) string {
	if cfg.Rootfs.Path != "" {
		return cfg.Rootfs.Path
	}
	return ""
}
