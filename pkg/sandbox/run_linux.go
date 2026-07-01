//go:build linux

package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func killPid(pid int) error { return syscall.Kill(pid, syscall.SIGKILL) }

// runLinuxCmd starts cmd in its own process group, optionally inside a cgroup
// (clone3 CLONE_INTO_CGROUP via SysProcAttr.UseCgroupFD, avoiding the "ran
// before it was placed" race), and enforces a wall-clock timeout by killing the
// whole cgroup (atomic) with a process-group SIGKILL backstop. It mirrors
// pkg/cmdx.RunTimeout but adds cgroup placement, which cmdx can't do because it
// hard-codes SysProcAttr.
func runLinuxCmd(cmd *exec.Cmd, timeout time.Duration, cg *cgroupHandle) (err error, timedOut bool) {
	spa := &syscall.SysProcAttr{Setpgid: true}
	if cg.ok() {
		spa.UseCgroupFD = true
		spa.CgroupFD = cg.fd()
	}
	cmd.SysProcAttr = spa

	if err := cmd.Start(); err != nil {
		return err, false
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-time.After(timeout):
		if cg.ok() {
			cg.kill()
		}
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		<-done
		return nil, true
	case e := <-done:
		return e, false
	}
}

// remapPath rewrites a host path to its in-sandbox target using the mount table
// (skill_runtime emits host paths in Command/Cwd/Env; engines that bind-mount —
// bwrap — must translate them to the canonical /skill,/workspace,... targets).
// Non-path strings pass through unchanged.
func remapPath(mounts []MountSpec, p string) string {
	for _, m := range mounts {
		if m.Source == "" || m.Target == "" {
			continue
		}
		if p == m.Source {
			return m.Target
		}
		if strings.HasPrefix(p, m.Source+string(filepath.Separator)) {
			return m.Target + p[len(m.Source):]
		}
	}
	return p
}

// cgroupKilledReason inspects the cgroup's event files after a run to attribute
// a kill to OOM or the pids limit (best-effort; "" when neither fired).
func cgroupKilledReason(cg *cgroupHandle) string {
	if !cg.ok() {
		return ""
	}
	if eventHasNonZero(filepath.Join(cg.dir, "memory.events"), "oom_kill") {
		return KilledByOOM
	}
	if eventHasNonZero(filepath.Join(cg.dir, "pids.events"), "max") {
		return KilledByPids
	}
	return ""
}

func eventHasNonZero(path, key string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == key && f[1] != "0" {
			return true
		}
	}
	return false
}
