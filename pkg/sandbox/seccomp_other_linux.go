//go:build linux && !amd64 && !arm64

package sandbox

// Other Linux architectures (arm, 386, ppc64le, riscv64, …) have no curated
// seccomp profile yet, so seccomp is skipped there (the engine still applies
// namespaces + cgroup). archSeccompToken returning ok=false drives that;
// allowedSyscalls is never reached but must exist for the build.
func archSeccompToken() (uint32, bool) { return 0, false }

func allowedSyscalls() []int { return nil }
