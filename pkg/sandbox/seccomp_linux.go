//go:build linux

package sandbox

import (
	"encoding/binary"
	"os"

	"golang.org/x/sys/unix"
)

// seccomp_linux.go builds a seccomp-bpf default-deny allow-list as a compiled
// cBPF program and hands it to bwrap via `--seccomp <fd>` (bwrap installs it on
// the payload just before exec, after its own setup). Pure Go, no CGO / no
// libseccomp, so the single-static-binary + cross-compile model is unchanged
// (§8.1). The allow-list is the union of a cross-arch common set (here) and a
// per-arch set (seccomp_{arm64,amd64}.go); unsupported arches return ok=false
// and the engine simply runs without seccomp (graceful degradation).
//
// Two modes (§15): "audit" returns SECCOMP_RET_LOG for denied syscalls (nothing
// is blocked — used first to collect the real syscall set from the kernel log),
// "enforce" returns EPERM. Foreign-architecture syscalls are always killed
// (classic 32-bit-on-64 bypass), in both modes.

// classic BPF instruction (struct sock_filter): {code, jt, jf, k}.
type sockFilter struct {
	code uint16
	jt   uint8
	jf   uint8
	k    uint32
}

// BPF opcodes (linux/bpf_common.h).
const (
	bpfLD  = 0x00
	bpfW   = 0x00
	bpfABS = 0x20
	bpfJMP = 0x05
	bpfJEQ = 0x10
	bpfK   = 0x00
	bpfRET = 0x06
)

// seccomp_data field offsets and return actions (linux/seccomp.h).
const (
	scNrOffset   = 0
	scArchOffset = 4

	retKillProcess = 0x80000000
	retErrno       = 0x00050000
	retLog         = 0x7ffc0000
	retAllow       = 0x7fff0000
)

// buildSeccompFilter compiles the allow-list into cBPF. ok=false means this
// architecture has no profile and seccomp should be skipped.
func buildSeccompFilter(enforce bool) (prog []sockFilter, ok bool) {
	arch, ok := archSeccompToken()
	if !ok {
		return nil, false
	}
	defaultAction := uint32(retLog)
	if enforce {
		defaultAction = retErrno | uint32(unix.EPERM)
	}

	prog = []sockFilter{
		// A = seccomp_data.arch
		{code: bpfLD | bpfW | bpfABS, k: scArchOffset},
		// if A == ourArch: skip the kill; else fall through to kill
		{code: bpfJMP | bpfJEQ | bpfK, k: arch, jt: 1, jf: 0},
		{code: bpfRET | bpfK, k: retKillProcess},
		// A = seccomp_data.nr
		{code: bpfLD | bpfW | bpfABS, k: scNrOffset},
	}
	for _, nr := range allowedSyscalls() {
		// if A == nr: fall through to ALLOW; else skip the ALLOW
		prog = append(prog,
			sockFilter{code: bpfJMP | bpfJEQ | bpfK, k: uint32(nr), jt: 0, jf: 1},
			sockFilter{code: bpfRET | bpfK, k: retAllow},
		)
	}
	prog = append(prog, sockFilter{code: bpfRET | bpfK, k: defaultAction})
	return prog, true
}

func serializeSeccomp(prog []sockFilter) []byte {
	b := make([]byte, 0, len(prog)*8)
	var tmp [8]byte
	for _, f := range prog {
		binary.LittleEndian.PutUint16(tmp[0:2], f.code)
		tmp[2] = f.jt
		tmp[3] = f.jf
		binary.LittleEndian.PutUint32(tmp[4:8], f.k)
		b = append(b, tmp[:]...)
	}
	return b
}

// seccompProgramReader returns the read end of a pipe preloaded with the
// compiled cBPF program, to be passed to bwrap as an inherited fd (--seccomp).
// The whole program (~a few KB) fits in the pipe buffer so the write completes
// without a reader. ok=false → no profile for this arch (skip seccomp). The
// caller owns the returned file and must Close it after the child starts.
func seccompProgramReader(mode string) (r *os.File, ok bool, err error) {
	if mode == "off" || mode == "disabled" {
		return nil, false, nil
	}
	prog, ok := buildSeccompFilter(mode == "enforce")
	if !ok {
		return nil, false, nil
	}
	data := serializeSeccomp(prog)
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, false, err
	}
	if _, err := pw.Write(data); err != nil {
		pw.Close()
		pr.Close()
		return nil, false, err
	}
	pw.Close()
	return pr, true, nil
}

// allowedSyscalls (the union of the cross-arch common set and per-arch extras)
// and the lists themselves live in the per-arch files: seccomp_lists_supported.go
// for amd64/arm64, seccomp_other_linux.go (returns nil) for the rest. Dangerous
// syscalls (mount/ptrace/bpf/perf_event_open/keyctl/userfaultfd/kexec/
// init_module/unshare/setns/pivot_root/chroot/process_vm_*…) are intentionally
// absent → they hit the default action (EPERM in enforce).
