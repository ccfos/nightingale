//go:build linux

package sandbox

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// probeCapabilities inspects the running kernel for the features the engines
// need (§6). It is best-effort and read-only: it reads sysctls / sysfs and
// looks up the bwrap binary, never mutating host state. Phase 2 may tighten the
// userns and cgroup-delegation checks (e.g. an actual unshare(2) attempt); the
// conservative reads here are enough to drive tier selection.
func probeCapabilities() Capabilities {
	c := baseCaps()
	c.KernelVersion = strings.TrimSpace(readFile("/proc/sys/kernel/osrelease"))

	c.UserNS = probeUserNS(&c)
	c.Seccomp = probeSeccomp(&c)
	c.Landlock = probeLandlock(&c)
	c.CgroupV2Delegated = probeCgroupV2(&c)

	if p, err := exec.LookPath("bwrap"); err == nil {
		c.BwrapPath = p
	} else {
		c.note("bubblewrap binary (bwrap) not found in PATH")
	}
	return c
}

// probeUserNS heuristically decides whether unprivileged user namespaces can be
// created. Distros gate this in different places; we treat an explicit "off"
// signal as authoritative and otherwise assume available.
func probeUserNS(c *Capabilities) bool {
	// Debian/Ubuntu knob: 0 means unprivileged userns is disabled.
	if v := strings.TrimSpace(readFile("/proc/sys/kernel/unprivileged_userns_clone")); v != "" {
		if v == "0" {
			c.note("unprivileged_userns_clone=0 (distro disabled unprivileged userns)")
			return false
		}
	}
	// Hard cap of 0 disables userns regardless of the above.
	if v := strings.TrimSpace(readFile("/proc/sys/user/max_user_namespaces")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n == 0 {
			c.note("user.max_user_namespaces=0 (userns disabled)")
			return false
		}
	}
	return true
}

func probeSeccomp(c *Capabilities) bool {
	// A "Seccomp:" line in /proc/self/status means the kernel supports seccomp.
	if strings.Contains(readFile("/proc/self/status"), "Seccomp:") {
		return true
	}
	c.note("seccomp not advertised in /proc/self/status")
	return false
}

func probeLandlock(c *Capabilities) bool {
	if strings.Contains(readFile("/sys/kernel/security/lsm"), "landlock") {
		return true
	}
	c.note("landlock not listed in active LSMs (need kernel 5.13+ with landlock enabled)")
	return false
}

func probeCgroupV2(c *Capabilities) bool {
	// cgroup v2 unified hierarchy exposes cgroup.controllers at the root.
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		return true
	}
	c.note("cgroup v2 unified hierarchy not detected at /sys/fs/cgroup")
	return false
}

// readFile returns the file contents or "" on any error (probe is best-effort).
func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}
