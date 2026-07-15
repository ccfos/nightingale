//go:build linux

package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/toolkits/pkg/logger"
)

// confinedEngine is the container-as-boundary degradation tier (档 0.5, §5.3):
// for container deployments where unprivileged userns is blocked (Docker
// default seccomp / Ubuntu 24.04 AppArmor) so bubblewrap can't start. It creates
// NO new namespaces — the outer container provides host isolation — and only
// layers the zero-privilege controls: drop caps, no_new_privs, seccomp,
// Landlock, rlimit. Because Go's os/exec offers no pre-exec hook to apply
// prctl/seccomp/Landlock in the child, this engine uses nsjail as its launcher
// (the design-sanctioned "nsjail with namespace clone disabled" approach), with
// cgroup limits applied by the control plane on top.
//
// VERIFICATION STATUS: cross-compiles and registers, but written without a
// Linux test host and the exact nsjail flag set must be validated on Linux
// (design §18.2). It is only selected when the operator sets
// container_as_boundary=true (never a silent downgrade, §6 档 0.5). Outstanding:
// a tuned Kafel seccomp policy + Landlock rules (currently relies on nsjail's
// defaults + the cgroup/rlimit layer + the outer container).
type confinedEngine struct {
	nsjail string
	cfg    Config
}

func init() {
	registerEngine(EngineConfined, func(cfg Config, caps Capabilities) (Engine, error) {
		if !cfg.ContainerAsBoundary {
			return nil, fmt.Errorf("container-confined requires container_as_boundary=true (explicit operator acknowledgement)")
		}
		path, err := exec.LookPath("nsjail")
		if err != nil {
			return nil, fmt.Errorf("nsjail binary not found (container-confined uses nsjail as its no-namespace launcher): %w", err)
		}
		return &confinedEngine{nsjail: path, cfg: cfg}, nil
	})
}

func (e *confinedEngine) Name() string { return EngineConfined }

func (e *confinedEngine) Caps() EngineCaps {
	// No namespaces (container provides them); seccomp + cgroup do apply.
	return EngineCaps{Seccomp: true, Cgroup: true}
}

func (e *confinedEngine) Run(ctx context.Context, spec ExecSpec) (ExecResult, error) {
	if len(spec.Command) == 0 {
		return ExecResult{}, fmt.Errorf("empty command")
	}
	cg := setupCgroup(spec.ExecID, spec.Resources)
	defer cg.cleanup()

	args := e.buildArgs(spec)
	logger.Debugf("sandbox[container-confined]: skill=%s argv=%v", spec.SkillName, spec.Command)

	cmd := exec.Command(e.nsjail, args...)
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
			return res, fmt.Errorf("nsjail exec: %w", runErr)
		}
	}
	return res, nil
}

// buildArgs builds an nsjail invocation that creates NO new namespaces (the
// container is the boundary) and applies the zero-privilege confinement layer.
// Command/Cwd use real host paths (no mount namespace, so no remapping).
func (e *confinedEngine) buildArgs(spec ExecSpec) []string {
	a := []string{
		"-Mo",     // execve once
		"--quiet", // keep nsjail's own logging off the captured stderr
		// Do not clone any new namespace — rely on the outer container.
		"--disable_clone_newnet",
		"--disable_clone_newuser",
		"--disable_clone_newns",
		"--disable_clone_newpid",
		"--disable_clone_newipc",
		"--disable_clone_newuts",
		"--disable_clone_newcgroup",
	}
	if spec.Resources.MemoryMB > 0 {
		a = append(a, "--rlimit_as", strconv.FormatInt(spec.Resources.MemoryMB, 10)) // MB
	}
	if spec.Resources.Pids > 0 {
		a = append(a, "--rlimit_nproc", strconv.FormatInt(spec.Resources.Pids, 10))
	}
	if spec.Cwd != "" {
		a = append(a, "--cwd", spec.Cwd)
	}
	for k, v := range spec.Env {
		a = append(a, "--env", k+"="+v)
	}
	a = append(a, "--")
	a = append(a, spec.Command...)
	return a
}
