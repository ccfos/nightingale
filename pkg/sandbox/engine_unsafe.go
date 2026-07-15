package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/cmdx"
	"github.com/toolkits/pkg/logger"
)

// unsafeEngine runs the command with a bare exec — NO isolation. It exists only
// to (a) prove the end-to-end control-plane wiring on any OS and (b) serve as
// the dev fallback when no kernel-isolation engine is available (§5.2). It still
// applies the cheap, OS-portable controls the control plane owns: a clean
// (non-inherited) env, a working dir, a wall-clock timeout that kills the whole
// process group, and byte-capped output. It is the fail-open floor of last
// resort (resolveEngine); set RequireIsolation=true to refuse it in production.
type unsafeEngine struct{}

func init() {
	registerEngine(EngineUnsafe, func(cfg Config, caps Capabilities) (Engine, error) {
		return &unsafeEngine{}, nil
	})
}

func (e *unsafeEngine) Name() string { return EngineUnsafe }

func (e *unsafeEngine) Caps() EngineCaps {
	// Bare exec enforces none of these.
	return EngineCaps{}
}

func (e *unsafeEngine) Run(ctx context.Context, spec ExecSpec) (ExecResult, error) {
	if len(spec.Command) == 0 {
		return ExecResult{}, fmt.Errorf("empty command")
	}
	logger.Warningf("sandbox[unsafe-exec]: running skill %q with NO ISOLATION (dev only) argv=%v", spec.SkillName, spec.Command)

	cmd := exec.Command(spec.Command[0], spec.Command[1:]...)
	cmd.Dir = spec.Cwd
	cmd.Env = buildEnv(spec.Env)
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
	// cmdx sets Setpgid + SIGKILLs the whole group on timeout.
	runErr, isTimeout := cmdx.RunTimeout(cmd, timeout)
	dur := time.Since(start)

	res := ExecResult{
		Stdout:          out.buf.Bytes(),
		Stderr:          errb.buf.Bytes(),
		StdoutTruncated: out.truncated,
		StderrTruncated: errb.truncated,
		Duration:        dur,
	}

	if isTimeout {
		res.Timeout = true
		res.KilledBy = KilledByTimeout
		res.ExitCode = -1
		res.Error = fmt.Sprintf("killed after %s timeout", timeout)
		return res, nil
	}

	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}
	if runErr != nil {
		// A non-zero exit is reported via ExitCode, not as a Run error. Only a
		// genuine start/exec failure (binary missing, etc.) is a Run error.
		if _, ok := runErr.(*exec.ExitError); !ok {
			return res, fmt.Errorf("exec %q: %w", spec.Command[0], runErr)
		}
	}
	return res, nil
}

// buildEnv returns the child environment: ONLY the whitelisted entries from the
// spec, never the host's os.Environ(). A minimal PATH is injected when the spec
// omits one so the interpreter can still find common helpers.
func buildEnv(env map[string]string) []string {
	out := make([]string, 0, len(env)+1)
	hasPath := false
	for k, v := range env {
		if k == "PATH" {
			hasPath = true
		}
		out = append(out, k+"="+v)
	}
	if !hasPath {
		out = append(out, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	}
	return out
}

// cappedBuffer accumulates up to max bytes and then silently drops the rest,
// flagging truncation. It returns len(p) even while dropping so the child's
// write never errors (which would kill the process before it finishes). max<=0
// means unbounded.
type cappedBuffer struct {
	buf       bytes.Buffer
	max       int64
	truncated bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if c.max <= 0 {
		return c.buf.Write(p)
	}
	remaining := c.max - int64(c.buf.Len())
	if remaining <= 0 {
		c.truncated = true
		return len(p), nil
	}
	if int64(len(p)) > remaining {
		c.buf.Write(p[:remaining])
		c.truncated = true
		return len(p), nil
	}
	return c.buf.Write(p)
}
