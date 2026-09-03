//go:build linux && arm64

package sandbox

import "golang.org/x/sys/unix"

// AUDIT_ARCH_AARCH64 (linux/audit.h) — pins the filter to arm64 so 32-bit /
// foreign-arch syscall ABIs are rejected (a classic seccomp bypass).
const auditArchAARCH64 = 0xC00000B7

func archSeccompToken() (uint32, bool) { return auditArchAARCH64, true }

// archExtraSyscalls: arm64-only / not-in-common names that python+bash still
// need. Most arm64 syscalls are covered by the common list; arm64 has no legacy
// open/poll/dup2/fork variants (it uses the *at / clone forms already allowed).
func archExtraSyscalls() []int {
	return []int{
		unix.SYS_CLONE3,
		unix.SYS_OPENAT2,
		unix.SYS_FACCESSAT2,
		unix.SYS_PIDFD_OPEN,
		unix.SYS_PIDFD_SEND_SIGNAL,
		unix.SYS_EPOLL_PWAIT2,
	}
}
