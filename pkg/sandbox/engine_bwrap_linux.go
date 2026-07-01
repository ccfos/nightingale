//go:build linux

package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
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
	bwrap    string
	base     string
	cfg      Config
	initPath string // n9e-sandbox-init forwarder, or "" → network=proxy unsupported
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
		// The egress forwarder (§10.2): prefer the embedded one extracted at
		// startup, fall back to an operator-supplied path. Empty is fine —
		// network=none still works; only network=proxy needs it.
		initPath := caps.InitPath
		if initPath == "" {
			initPath = cfg.EgressProxy.ForwarderPath
		}
		return &bwrapEngine{bwrap: caps.BwrapPath, base: base, cfg: cfg, initPath: initPath}, nil
	})
}

func (e *bwrapEngine) Name() string { return EngineBwrap }

func (e *bwrapEngine) Caps() EngineCaps {
	c := EngineCaps{Namespaces: true, Cgroup: true}
	// Egress (network=proxy) needs the in-netns forwarder; without it bwrap can
	// still isolate (network=none) but cannot enforce a proxy egress posture.
	c.Network = e.initPath != ""
	if _, ok := archSeccompToken(); ok && e.cfg.SeccompMode != "" && e.cfg.SeccompMode != "off" {
		c.Seccomp = true
	}
	return c
}

func (e *bwrapEngine) Run(ctx context.Context, spec ExecSpec) (ExecResult, error) {
	if len(spec.Command) == 0 {
		return ExecResult{}, fmt.Errorf("empty command")
	}
	if spec.Network == NetworkProxy && e.initPath == "" {
		return ExecResult{}, fmt.Errorf("network=proxy requires the n9e-sandbox-init forwarder: build with -tags sandbox_embed or set Sandbox.EgressProxy.ForwarderPath")
	}
	cg := setupCgroup(spec.ExecID, spec.Resources)
	defer cg.cleanup()

	// Compile the seccomp filter and hand it to bwrap as an inherited fd. The
	// first ExtraFiles entry is fd 3 in the child; bwrap installs the filter on
	// the payload just before exec (after its own privileged setup).
	var extraFiles []*os.File
	seccompFD := -1
	if r, ok, serr := seccompProgramReader(e.cfg.SeccompMode); serr != nil {
		logger.Warningf("sandbox[bwrap]: seccomp build failed, running WITHOUT seccomp: %v", serr)
	} else if ok {
		extraFiles = append(extraFiles, r)
		seccompFD = 3 // ExtraFiles[0]
		defer r.Close()
	}

	args := e.buildArgs(spec, seccompFD)
	logger.Debugf("sandbox[bwrap]: skill=%s seccomp=%s seccompFD=%d argv=%v", spec.SkillName, e.cfg.SeccompMode, seccompFD, spec.Command)

	cmd := exec.Command(e.bwrap, args...)
	cmd.ExtraFiles = extraFiles
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
// /workspace /output, and /run for the control-plane tmpfs) under it — it fails
// with "Can't mkdir /skill: Read-only file system". The python-base rootfs MUST
// therefore ship those mount-point directories as empty dirs. The embedded-base
// extraction creates them (rootfs_extract.go); an externally-supplied
// Rootfs.Path must contain them too — including /run for network=proxy.
func (e *bwrapEngine) buildArgs(spec ExecSpec, seccompFD int) []string {
	a := []string{
		"--unshare-user", "--unshare-pid", "--unshare-ipc", "--unshare-uts", "--unshare-cgroup",
		"--die-with-parent", "--new-session",
		"--clearenv",
		"--ro-bind", e.base, "/",
		"--proc", "/proc",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
	}
	// Control-plane channels (egress proxy + Skill Gateway sockets, the egress
	// forwarder binary) live on a private writable tmpfs at /run: the base rootfs
	// is read-only, so the bind targets need a writable mount to be created onto
	// (§10.2/§12.1). Only mounted when there is something to host.
	useControl := spec.Network == NetworkProxy || len(spec.ControlMounts) > 0
	if useControl {
		a = append(a, "--tmpfs", controlTmpfs)
	}
	if seccompFD >= 0 {
		// bwrap reads the compiled cBPF program from this inherited fd and
		// installs it on the payload right before exec.
		a = append(a, "--seccomp", strconv.Itoa(seccompFD))
	}
	// network=none AND network=proxy both run in a private, loopback-only netns —
	// proxy never gets real host network, it reaches out only via the forwarder →
	// UDS → host egress proxy (§10.2). Only direct (future) keeps host network.
	if spec.Network != NetworkDirect {
		a = append(a, "--unshare-net")
	}
	if spec.Network == NetworkProxy {
		a = append(a, "--ro-bind", e.initPath, initTargetPath)
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
	// Control sockets are bound read-write (a client must be able to connect) and,
	// unlike user mounts, are never used to remap Command/Cwd/Env paths.
	for _, m := range spec.ControlMounts {
		if m.Source == "" || m.Target == "" {
			continue
		}
		a = append(a, "--bind", m.Source, m.Target)
	}
	for k, v := range spec.Env {
		a = append(a, "--setenv", k, remapPath(spec.Mounts, v))
	}
	if cwd := remapPath(spec.Mounts, spec.Cwd); cwd != "" {
		a = append(a, "--chdir", cwd)
	}
	a = append(a, "--")
	// Under network=proxy the payload is launched THROUGH the forwarder/init,
	// which brings up the loopback proxy listener (HTTP_PROXY target) and then
	// execs the skill (§10.2). Under none/direct the skill is the payload directly.
	if spec.Network == NetworkProxy {
		a = append(a, initTargetPath, "--forward", EgressForwarderListen+"="+EgressSocketTarget, "--")
	}
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
