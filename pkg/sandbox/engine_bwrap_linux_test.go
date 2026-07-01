//go:build linux

package sandbox

import (
	"strings"
	"testing"
)

// hasSeq reports whether sub appears as a contiguous subsequence of args.
func hasSeq(args []string, sub ...string) bool {
	for i := 0; i+len(sub) <= len(args); i++ {
		if strings.Join(args[i:i+len(sub)], "\x00") == strings.Join(sub, "\x00") {
			return true
		}
	}
	return false
}

func TestBwrapBuildArgsNetworkNone(t *testing.T) {
	e := &bwrapEngine{bwrap: "/bin/bwrap", base: "/base", initPath: "/host/init"}
	args := e.buildArgs(ExecSpec{
		Network: NetworkNone,
		Command: []string{"python3", "/skill/main.py"},
		Mounts:  []MountSpec{{Source: "/ws/skill", Target: "/skill", ReadOnly: true}},
	}, -1)

	if !hasSeq(args, "--unshare-net") {
		t.Error("network=none must still unshare the net namespace")
	}
	if hasSeq(args, "--tmpfs", controlTmpfs) {
		t.Error("no control tmpfs should be mounted without proxy/control mounts")
	}
	if hasSeq(args, "--ro-bind", "/host/init", initTargetPath) {
		t.Error("forwarder must not be bound when network=none")
	}
	// Payload runs directly, not through the forwarder launcher.
	if !hasSeq(args, "--", "python3", "/skill/main.py") {
		t.Errorf("expected direct payload, got %v", args)
	}
	if hasSeq(args, initTargetPath, "--forward") {
		t.Error("forwarder launcher must not prefix the command for network=none")
	}
}

func TestBwrapBuildArgsNetworkProxy(t *testing.T) {
	e := &bwrapEngine{bwrap: "/bin/bwrap", base: "/base", initPath: "/host/init"}
	args := e.buildArgs(ExecSpec{
		Network: NetworkProxy,
		Command: []string{"python3", "/skill/main.py"},
		Mounts:  []MountSpec{{Source: "/ws/skill", Target: "/skill", ReadOnly: true}},
		ControlMounts: []MountSpec{
			{Source: "/host/egress.sock", Target: EgressSocketTarget},
			{Source: "/host/gw.sock", Target: GatewaySocketTarget},
		},
	}, -1)

	checks := [][]string{
		{"--unshare-net"},                                   // still isolated
		{"--tmpfs", controlTmpfs},                           // /run tmpfs for control plane
		{"--ro-bind", "/host/init", initTargetPath},         // forwarder binary
		{"--bind", "/host/egress.sock", EgressSocketTarget}, // egress socket
		{"--bind", "/host/gw.sock", GatewaySocketTarget},    // gateway socket
		// launcher: init --forward LISTEN=SOCK -- <cmd>
		{"--", initTargetPath, "--forward", EgressForwarderListen + "=" + EgressSocketTarget, "--", "python3", "/skill/main.py"},
	}
	for _, c := range checks {
		if !hasSeq(args, c...) {
			t.Errorf("proxy argv missing %v\nfull: %v", c, args)
		}
	}
}

func TestBwrapBuildArgsGatewayOnlyNoForwarder(t *testing.T) {
	// network=none but with a gateway control mount: /run tmpfs + the socket bind,
	// but NO forwarder binary and NO launcher prefix.
	e := &bwrapEngine{bwrap: "/bin/bwrap", base: "/base", initPath: "/host/init"}
	args := e.buildArgs(ExecSpec{
		Network:       NetworkNone,
		Command:       []string{"bash", "/skill/run.sh"},
		ControlMounts: []MountSpec{{Source: "/host/gw.sock", Target: GatewaySocketTarget}},
	}, -1)

	if !hasSeq(args, "--tmpfs", controlTmpfs) {
		t.Error("gateway control mount requires the /run tmpfs")
	}
	if !hasSeq(args, "--bind", "/host/gw.sock", GatewaySocketTarget) {
		t.Error("gateway socket must be bound")
	}
	if hasSeq(args, "--ro-bind", "/host/init", initTargetPath) || hasSeq(args, initTargetPath, "--forward") {
		t.Error("no forwarder/launcher should appear when network=none")
	}
}
