//go:build linux

package sandbox

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/toolkits/pkg/logger"
)

// cgroup v2 limits are a CONTROL-PLANE responsibility, applied identically
// regardless of which engine runs underneath (§14.1). This manager creates a
// per-execution cgroup v2 subtree under a writable parent, writes
// memory/pids/cpu limits, hands the engine a directory fd to place the child via
// clone3(CLONE_INTO_CGROUP) (exec.Cmd.SysProcAttr.UseCgroupFD), and kills the
// whole tree on timeout (cgroup.kill). Everything is best-effort: on a host
// where no writable cgroup parent exists the limits silently degrade to
// rlimit + timeout (§17), the run still proceeds.
//
// Picking the parent (resolveCgroupParent, done once):
//   - root / root-in-container: create n9e-sandbox directly under the cgroup
//     root (the root cgroup is exempt from the "no internal processes" rule).
//   - non-root with a DELEGATED cgroup (e.g. a systemd user service / scope with
//     Delegate=yes): nest under n9e's OWN cgroup. cgroup v2 forbids a non-root
//     cgroup from both holding processes and delegating controllers, so we first
//     move n9e itself into a "sb-supervisor" leaf, freeing the parent to enable
//     controllers and host the per-exec cgroups.
//   - otherwise (non-root, no delegation): no writable parent → limits disabled.
const (
	cgroupRoot   = "/sys/fs/cgroup"
	cgroupParent = "n9e-sandbox"
	supervisor   = "sb-supervisor"
)

var (
	cgParentOnce sync.Once
	cgParentDir  string // resolved writable parent, or "" if none
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

// resolveCgroupParent finds (once) the cgroup directory under which per-exec
// cgroups can be created with memory/pids/cpu limits, or "" if none is usable.
func resolveCgroupParent() string {
	cgParentOnce.Do(func() { cgParentDir = doResolveCgroupParent() })
	return cgParentDir
}

func doResolveCgroupParent() string {
	if _, err := os.Stat(filepath.Join(cgroupRoot, "cgroup.controllers")); err != nil {
		logger.Warningf("sandbox cgroup: cgroup v2 unified hierarchy not found; resource limits disabled")
		return "" // not cgroup v2 unified
	}
	// 1) Root-level: works for root / root-in-container (cgroup root is exempt
	//    from the no-internal-process rule). No supervisor move needed.
	if p, ok := prepareParent(filepath.Join(cgroupRoot, cgroupParent), false); ok {
		logger.Infof("sandbox cgroup: per-exec parent = %s (root-level)", p)
		return p
	}
	// 2) Own delegated cgroup: works for non-root with Delegate=yes. Needs the
	//    supervisor move so the parent can delegate controllers.
	if own := ownCgroupDir(); own != "" {
		if p, ok := prepareParent(own, true); ok {
			logger.Infof("sandbox cgroup: per-exec parent = %s (own delegated cgroup + supervisor)", p)
			return p
		}
	}
	logger.Warningf("sandbox cgroup: no writable cgroup parent — resource limits DISABLED " +
		"(run n9e as root, in a container, or as a systemd service with Delegate=yes)")
	return ""
}

// ownCgroupDir returns n9e's own cgroup v2 directory from /proc/self/cgroup, or
// "" when n9e sits at the cgroup root (handled by the root-level path).
func ownCgroupDir() string {
	b, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if rel, ok := strings.CutPrefix(line, "0::"); ok { // v2 unified line
			rel = strings.TrimSpace(rel)
			if rel == "" || rel == "/" {
				return "" // at root → root-level path handles it
			}
			return filepath.Join(cgroupRoot, rel)
		}
	}
	return ""
}

// prepareParent makes dir usable as a per-exec parent: optionally moves n9e into
// a supervisor leaf (moveSelf, for the non-root own-cgroup case), delegates the
// controllers down, and verifies dir now hands memory/pids to its children.
func prepareParent(dir string, moveSelf bool) (string, bool) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false
	}
	if moveSelf && !ensureSupervisor(dir) {
		return "", false
	}
	// Best-effort: ask the parent to delegate controllers to dir (helps the
	// root path when the root hasn't delegated yet), then enable them on dir so
	// its per-exec children inherit them.
	enableAvailableControllers(filepath.Dir(dir))
	enableAvailableControllers(dir)

	sc := readFile(filepath.Join(dir, "cgroup.subtree_control"))
	if strings.Contains(sc, "memory") || strings.Contains(sc, "pids") {
		return dir, true
	}
	return "", false
}

// ensureSupervisor moves every process in x into x/sb-supervisor so x becomes an
// internal node that can delegate controllers (cgroup v2 no-internal-process
// rule). Idempotent. Returns false if x still holds processes afterwards.
func ensureSupervisor(x string) bool {
	sup := filepath.Join(x, supervisor)
	if err := os.MkdirAll(sup, 0o755); err != nil {
		logger.Warningf("sandbox cgroup: mkdir supervisor %s failed: %v", sup, err)
		return false
	}
	procs := filepath.Join(x, "cgroup.procs")
	supProcs := filepath.Join(sup, "cgroup.procs")
	for _, pid := range strings.Fields(readFile(procs)) {
		if err := writeFileRaw(supProcs, pid); err != nil {
			logger.Warningf("sandbox cgroup: move pid %s into supervisor failed: %v", pid, err)
		}
	}
	if remaining := strings.Fields(readFile(procs)); len(remaining) > 0 {
		logger.Warningf("sandbox cgroup: %s still has %d processes after supervisor move; cannot delegate",
			x, len(remaining))
		return false
	}
	return true
}

// enableAvailableControllers turns on (in dir's subtree_control) each of
// memory/pids/cpu that dir actually has available, one at a time so a missing
// controller doesn't block the others. Best-effort.
func enableAvailableControllers(dir string) {
	avail := readFile(filepath.Join(dir, "cgroup.controllers"))
	for _, c := range []string{"memory", "pids", "cpu"} {
		if strings.Contains(avail, c) {
			_ = writeFileRaw(filepath.Join(dir, "cgroup.subtree_control"), "+"+c)
		}
	}
}

// setupCgroup creates and configures the per-exec subtree. It never returns an
// error: failure yields a no-op handle (ok()==false) and a logged warning.
func setupCgroup(execID string, res ResourceSpec) *cgroupHandle {
	parent := resolveCgroupParent()
	if parent == "" {
		return &cgroupHandle{}
	}

	dir := filepath.Join(parent, execID)
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

// cleanup closes the fd and removes the per-exec subtree (must be empty — call
// after the child has exited / been killed). The shared parent + supervisor are
// left in place for reuse across executions.
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
