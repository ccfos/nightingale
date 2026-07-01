//go:build linux && amd64

package sandbox

import "golang.org/x/sys/unix"

// AUDIT_ARCH_X86_64 (linux/audit.h). NOTE: this amd64 allow-list is written but
// has NOT been exercised on an amd64 host (the live test ran on arm64). Verify
// with `seccomp_mode=audit` on amd64 before enforcing.
const auditArchX8664 = 0xC000003E

func archSeccompToken() (uint32, bool) { return auditArchX8664, true }

// archExtraSyscalls: amd64-only legacy names (the arch still ships open/poll/
// dup2/… and needs arch_prctl for TLS) plus the newest syscalls.
func archExtraSyscalls() []int {
	return []int{
		unix.SYS_OPEN, unix.SYS_STAT, unix.SYS_LSTAT, unix.SYS_POLL,
		unix.SYS_ACCESS, unix.SYS_PIPE, unix.SYS_SELECT, unix.SYS_DUP2,
		unix.SYS_RENAME, unix.SYS_MKDIR, unix.SYS_RMDIR, unix.SYS_UNLINK,
		unix.SYS_SYMLINK, unix.SYS_LINK, unix.SYS_READLINK, unix.SYS_CHMOD,
		unix.SYS_CHOWN, unix.SYS_LCHOWN, unix.SYS_GETDENTS,
		unix.SYS_EPOLL_CREATE, unix.SYS_EVENTFD, unix.SYS_ACCEPT,
		unix.SYS_ARCH_PRCTL, unix.SYS_FORK, unix.SYS_VFORK,
		unix.SYS_CREAT, unix.SYS_CLONE3, unix.SYS_OPENAT2, unix.SYS_FACCESSAT2,
		unix.SYS_PIDFD_OPEN, unix.SYS_PIDFD_SEND_SIGNAL, unix.SYS_EPOLL_PWAIT2,
	}
}
