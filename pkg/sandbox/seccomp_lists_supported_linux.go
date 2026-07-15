//go:build linux && (amd64 || arm64)

package sandbox

import "golang.org/x/sys/unix"

// allowedSyscalls is the union of the cross-arch common set and the per-arch
// extras (archExtraSyscalls, in seccomp_{arm64,amd64}.go). Only compiled for
// amd64/arm64 — other linux arches use the seccomp_other_linux.go stub.
func allowedSyscalls() []int {
	return append(commonAllowedSyscalls(), archExtraSyscalls()...)
}

// commonAllowedSyscalls: syscalls present (by this name) on both arm64 and
// amd64 in x/sys/unix. Anything name-divergent lives in the per-arch files.
func commonAllowedSyscalls() []int {
	return []int{
		unix.SYS_READ, unix.SYS_WRITE, unix.SYS_CLOSE, unix.SYS_LSEEK,
		unix.SYS_READV, unix.SYS_WRITEV, unix.SYS_PREAD64, unix.SYS_PWRITE64,
		unix.SYS_PREADV, unix.SYS_PWRITEV, unix.SYS_PREADV2, unix.SYS_PWRITEV2,
		unix.SYS_OPENAT, unix.SYS_CLOSE_RANGE, unix.SYS_FCNTL, unix.SYS_IOCTL,
		unix.SYS_GETDENTS64, unix.SYS_GETCWD, unix.SYS_CHDIR, unix.SYS_FCHDIR,
		unix.SYS_RENAMEAT, unix.SYS_RENAMEAT2, unix.SYS_MKDIRAT, unix.SYS_UNLINKAT,
		unix.SYS_SYMLINKAT, unix.SYS_LINKAT, unix.SYS_READLINKAT,
		unix.SYS_FCHMOD, unix.SYS_FCHMODAT, unix.SYS_FCHOWN, unix.SYS_FCHOWNAT,
		unix.SYS_UMASK, unix.SYS_FSTAT, unix.SYS_NEWFSTATAT, unix.SYS_STATX,
		unix.SYS_STATFS, unix.SYS_FSTATFS, unix.SYS_FACCESSAT,
		unix.SYS_FSYNC, unix.SYS_FDATASYNC, unix.SYS_FTRUNCATE, unix.SYS_FALLOCATE,
		unix.SYS_FADVISE64, unix.SYS_FLOCK, unix.SYS_SYNC,
		// Zero-copy data movement between fds. Go's net poller uses splice() for
		// conn→conn copies (the egress forwarder relies on it) and sendfile() for
		// file→socket; Python skills use them too. Benign (no new access) and in
		// Docker's default profile. Without splice the forwarder relay gets EPERM
		// and moves zero bytes (verified on a real sandbox, 2026-06-25).
		unix.SYS_SPLICE, unix.SYS_SENDFILE,
		unix.SYS_MMAP, unix.SYS_MUNMAP, unix.SYS_MPROTECT, unix.SYS_MREMAP,
		unix.SYS_MADVISE, unix.SYS_MSYNC, unix.SYS_MLOCK, unix.SYS_MUNLOCK, unix.SYS_BRK,
		unix.SYS_RT_SIGACTION, unix.SYS_RT_SIGPROCMASK, unix.SYS_RT_SIGRETURN,
		unix.SYS_RT_SIGTIMEDWAIT, unix.SYS_RT_SIGSUSPEND, unix.SYS_RT_SIGPENDING,
		unix.SYS_RT_SIGQUEUEINFO, unix.SYS_SIGALTSTACK,
		unix.SYS_KILL, unix.SYS_TKILL, unix.SYS_TGKILL,
		unix.SYS_NANOSLEEP, unix.SYS_CLOCK_NANOSLEEP, unix.SYS_CLOCK_GETTIME,
		unix.SYS_CLOCK_GETRES, unix.SYS_GETTIMEOFDAY,
		unix.SYS_GETITIMER, unix.SYS_SETITIMER,
		unix.SYS_FUTEX, unix.SYS_SET_TID_ADDRESS, unix.SYS_SET_ROBUST_LIST,
		unix.SYS_GET_ROBUST_LIST, unix.SYS_RSEQ,
		unix.SYS_SCHED_GETAFFINITY, unix.SYS_SCHED_SETAFFINITY, unix.SYS_SCHED_YIELD,
		unix.SYS_SCHED_GETPARAM, unix.SYS_SCHED_GETSCHEDULER,
		unix.SYS_GETPRIORITY, unix.SYS_SETPRIORITY,
		unix.SYS_PRLIMIT64, unix.SYS_GETRUSAGE, unix.SYS_UNAME, unix.SYS_SYSINFO,
		unix.SYS_EXIT, unix.SYS_EXIT_GROUP, unix.SYS_WAIT4, unix.SYS_WAITID,
		unix.SYS_CLONE, unix.SYS_EXECVE, unix.SYS_EXECVEAT, unix.SYS_PRCTL,
		unix.SYS_GETPID, unix.SYS_GETPPID, unix.SYS_GETUID, unix.SYS_GETEUID,
		unix.SYS_GETGID, unix.SYS_GETEGID, unix.SYS_GETTID,
		unix.SYS_GETPGID, unix.SYS_SETPGID, unix.SYS_GETSID, unix.SYS_SETSID,
		unix.SYS_GETRANDOM, unix.SYS_MEMFD_CREATE, unix.SYS_MEMBARRIER,
		unix.SYS_DUP, unix.SYS_DUP3, unix.SYS_PIPE2,
		unix.SYS_EPOLL_CREATE1, unix.SYS_EPOLL_CTL, unix.SYS_EPOLL_PWAIT,
		unix.SYS_EVENTFD2, unix.SYS_PPOLL, unix.SYS_PSELECT6,
		unix.SYS_TIMERFD_CREATE, unix.SYS_TIMERFD_SETTIME, unix.SYS_TIMERFD_GETTIME,
		unix.SYS_SOCKET, unix.SYS_SOCKETPAIR, unix.SYS_CONNECT, unix.SYS_BIND,
		unix.SYS_LISTEN, unix.SYS_ACCEPT4, unix.SYS_SENDTO, unix.SYS_RECVFROM,
		unix.SYS_SENDMSG, unix.SYS_RECVMSG, unix.SYS_SENDMMSG, unix.SYS_RECVMMSG,
		unix.SYS_SHUTDOWN, unix.SYS_GETSOCKNAME, unix.SYS_GETPEERNAME,
		unix.SYS_SETSOCKOPT, unix.SYS_GETSOCKOPT,
	}
}
