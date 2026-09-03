// Command n9e-sandbox-init is the tiny PID-1 launcher + egress forwarder that
// runs INSIDE the bubblewrap sandbox when network=proxy (design §10.2). It is the
// Go replacement for the socat relay Claude Code uses — shipped as one static,
// dependency-light binary (embedded in n9e via -tags sandbox_embed, or supplied
// externally) so the host needs no socat.
//
// It does two jobs and nothing else:
//
//  1. Forwarder: for each `--forward LISTEN=UDS`, listen on the loopback TCP
//     address LISTEN (inside the sandbox's private netns, where the script's
//     HTTP_PROXY points) and bridge every connection to the bind-mounted host
//     UDS, which reaches the out-of-sandbox egress proxy. Stock HTTP clients
//     speak TCP, not UNIX sockets, so this bridge is what makes them "just work".
//
//  2. Init: exec the real skill command as a child, pass through stdio + env,
//     reap incidental zombies (it is PID 1 of the sandbox pid-namespace), and
//     exit with the child's status — at which point the kernel tears down the
//     rest of the pid-namespace (killing the forwarder goroutines and any
//     orphans). Resource/timeout kills are handled externally by the control
//     plane's cgroup, so this stays minimal.
//
// Usage (built by the engine, never by humans):
//
//	n9e-sandbox-init --forward 127.0.0.1:18080=/run/n9e-egress.sock -- python3 /skill/main.py
//
// Build: CGO_ENABLED=0 go build ./cmd/n9e-sandbox-init  (fully static).
package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ccfos/nightingale/v6/pkg/sandbox/relay"
)

func main() {
	forwards, child, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "n9e-sandbox-init:", err)
		os.Exit(2)
	}
	if len(child) == 0 {
		fmt.Fprintln(os.Stderr, "n9e-sandbox-init: no child command after --")
		os.Exit(2)
	}

	for _, f := range forwards {
		if err := startForward(f); err != nil {
			// A forwarder that cannot bind means egress is broken; fail loudly
			// rather than silently running the skill with a dead proxy.
			fmt.Fprintln(os.Stderr, "n9e-sandbox-init: forward setup failed:", err)
			os.Exit(2)
		}
	}

	os.Exit(runChild(child))
}

// parseArgs splits "--forward A=B ... -- cmd args" into the forward specs and the
// child argv. Unknown flags before "--" are an error (the engine controls argv).
func parseArgs(args []string) (forwards []string, child []string, err error) {
	i := 0
	for i < len(args) {
		switch {
		case args[i] == "--":
			return forwards, args[i+1:], nil
		case args[i] == "--forward" && i+1 < len(args):
			forwards = append(forwards, args[i+1])
			i += 2
		default:
			return nil, nil, fmt.Errorf("unexpected argument %q", args[i])
		}
	}
	return forwards, nil, nil
}

// startForward parses "LISTEN=UDS", binds the loopback TCP listener, and relays
// every accepted connection to a fresh dial of the host UDS.
func startForward(spec string) error {
	listen, uds, ok := strings.Cut(spec, "=")
	if !ok || listen == "" || uds == "" {
		return fmt.Errorf("bad --forward %q (want LISTEN=UDS)", spec)
	}
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return fmt.Errorf("listen %s: %w", listen, err)
	}
	go func() {
		_ = relay.Serve(ln, func() (net.Conn, error) {
			return net.Dial("unix", uds)
		})
	}()
	return nil
}

// runChild starts the skill, then reaps children until the skill itself exits,
// returning its exit code. As PID 1 it adopts orphans; reaping them along the way
// keeps the pid table clean, and exiting on the skill's own exit lets the kernel
// SIGKILL whatever is left in the namespace.
func runChild(argv []string) int {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "n9e-sandbox-init: exec child:", err)
		return 127
	}
	childPID := cmd.Process.Pid

	// Forward termination signals to the whole child group so an external SIGTERM
	// stops the skill promptly (cgroup.kill is the hard backstop).
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		for range sigc {
			_ = syscall.Kill(-childPID, syscall.SIGTERM)
		}
	}()

	for {
		var ws syscall.WaitStatus
		pid, err := syscall.Wait4(-1, &ws, 0, nil)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			// ECHILD or similar: nothing left to wait on.
			return 0
		}
		if pid == childPID {
			return exitCode(ws)
		}
		// else: an orphan zombie was reaped; keep waiting for the skill.
	}
}

func exitCode(ws syscall.WaitStatus) int {
	if ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return ws.ExitStatus()
}
