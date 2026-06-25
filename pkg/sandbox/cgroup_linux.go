//go:build linux

package sandbox

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/toolkits/pkg/logger"
)

// cgroup v2 limits are a CONTROL-PLANE responsibility, applied identically
// regardless of which engine runs underneath (§14.1). This manager creates a
// per-execution cgroup v2 subtree, writes memory/pids/cpu limits, hands the
// engine a directory fd to place the child via clone3(CLONE_INTO_CGROUP)
// (exec.Cmd.SysProcAttr.UseCgroupFD), and kills the whole tree on timeout
// (cgroup.kill). Everything is best-effort: on a host without delegable cgroup
// v2 the limits silently degrade to rlimit + timeout (§17), the run still
// proceeds.
const (
	cgroupRoot   = "/sys/fs/cgroup"
	cgroupParent = "n9e-sandbox"
)

type cgroupHandle struct {
	dir string
	f   *os.File // kept open so .Fd() stays valid until after cmd.Start()
}

// fd returns the directory fd for CgroupFD, or -1 when no cgroup is active.
func (h *cgroupHandle) fd() int {
	if h == nil || h.f == nil {
		return -1
	}
	return int(h.f.Fd())
}

func (h *cgroupHandle) ok() bool { return h != nil && h.f != nil }

// setupCgroup creates and configures the subtree. It never returns an error:
// failure yields a no-op handle (ok()==false) and a logged warning.
func setupCgroup(execID string, res ResourceSpec) *cgroupHandle {
	base := filepath.Join(cgroupRoot, cgroupParent)
	if _, err := os.Stat(filepath.Join(cgroupRoot, "cgroup.controllers")); err != nil {
		return &cgroupHandle{} // not cgroup v2 unified
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		logger.Warningf("sandbox cgroup: mkdir %s failed (limits disabled): %v", base, err)
		return &cgroupHandle{}
	}
	// Delegate the controllers we need into the subtree (best-effort).
	enableControllers(base)

	dir := filepath.Join(base, execID)
	if err := os.Mkdir(dir, 0o755); err != nil && !os.IsExist(err) {
		logger.Warningf("sandbox cgroup: mkdir %s failed (limits disabled): %v", dir, err)
		return &cgroupHandle{}
	}

	if res.MemoryMB > 0 {
		writeCgroup(dir, "memory.max", strconv.FormatInt(res.MemoryMB*1024*1024, 10))
		writeCgroup(dir, "memory.swap.max", "0") // no swap, else OOM behaviour drifts
	}
	if res.Pids > 0 {
		writeCgroup(dir, "pids.max", strconv.FormatInt(res.Pids, 10))
	}
	if strings.TrimSpace(res.CPUQuota) != "" {
		writeCgroup(dir, "cpu.max", res.CPUQuota)
	}

	f, err := os.Open(dir)
	if err != nil {
		logger.Warningf("sandbox cgroup: open %s for fd failed: %v", dir, err)
		return &cgroupHandle{dir: dir}
	}
	return &cgroupHandle{dir: dir, f: f}
}

// kill terminates every process in the cgroup. cgroup.kill (5.14+) is atomic;
// otherwise fall back to SIGKILLing each pid in cgroup.procs.
func (h *cgroupHandle) kill() {
	if h == nil || h.dir == "" {
		return
	}
	if writeCgroup(h.dir, "cgroup.kill", "1") == nil {
		return
	}
	if b, err := os.ReadFile(filepath.Join(h.dir, "cgroup.procs")); err == nil {
		for _, line := range strings.Fields(string(b)) {
			if pid, err := strconv.Atoi(line); err == nil {
				_ = killPid(pid)
			}
		}
	}
}

// cleanup closes the fd and removes the subtree (must be empty — call after the
// child has exited / been killed).
func (h *cgroupHandle) cleanup() {
	if h == nil {
		return
	}
	if h.f != nil {
		_ = h.f.Close()
	}
	if h.dir != "" {
		_ = os.Remove(h.dir)
	}
}

func enableControllers(base string) {
	// Writing to the PARENT's subtree_control delegates controllers to children.
	parentSC := filepath.Join(filepath.Dir(base), "cgroup.subtree_control")
	_ = writeFileRaw(parentSC, "+memory +pids +cpu")
	_ = writeFileRaw(filepath.Join(base, "cgroup.subtree_control"), "+memory +pids +cpu")
}

func writeCgroup(dir, file, val string) error {
	if err := writeFileRaw(filepath.Join(dir, file), val); err != nil {
		logger.Warningf("sandbox cgroup: write %s=%s failed: %v", file, val, err)
		return err
	}
	return nil
}

func writeFileRaw(path, val string) error {
	return os.WriteFile(path, []byte(val), 0o644)
}
