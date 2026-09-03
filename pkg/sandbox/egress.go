package sandbox

// egress.go holds the conventions shared between the control plane (which starts
// the host-side egress proxy + injects HTTP_PROXY and binds the sockets) and the
// bubblewrap engine (which wraps the payload with the in-netns forwarder and
// binds the same paths). Keeping them in one place keeps both sides in agreement
// (§10.2). The data path for network=proxy:
//
//	script (HTTP_PROXY=http://127.0.0.1:P, stock requests/curl)
//	  → [sandbox netns] forwarder listens 127.0.0.1:P              (loopback TCP)
//	  → /run/n9e-egress.sock                                       (bind-mounted UDS)
//	  → [host] egress proxy: allowlist / DNS-pin / SSRF / audit    (egress_proxy.go)
//	  → external service
//
// The forwarder is the n9e-sandbox-init helper (cmd/n9e-sandbox-init): a tiny
// static Go relay that plays the socat role Claude Code uses, but shipped as one
// embedded binary so the host needs no external socat (§10.2 note).
const (
	// EgressForwarderListen is the loopback address the in-netns forwarder
	// listens on and that HTTP_PROXY/HTTPS_PROXY point at. It lives inside the
	// sandbox's private (loopback-only) network namespace, so the port never
	// collides with anything on the host or in other sandboxes.
	EgressForwarderListen = "127.0.0.1:18080"

	// EgressSocketTarget is where the host egress-proxy UDS is bind-mounted
	// inside the sandbox. The forwarder dials it; the script never sees it.
	EgressSocketTarget = "/run/n9e-egress.sock"

	// GatewaySocketTarget is where the per-exec Skill Gateway UDS is bind-mounted
	// inside the sandbox (§12.1). The script connects to it directly (AF_UNIX is
	// file IPC, not network — it works under network=none too).
	GatewaySocketTarget = "/run/n9e-skill-gateway.sock"

	// GatewaySocketEnv names the env var that tells the script where the gateway
	// socket is, so a skill SDK / raw AF_UNIX client can find it without guessing.
	GatewaySocketEnv = "N9E_SKILL_GATEWAY"

	// initTargetPath is where the n9e-sandbox-init forwarder binary is bind-
	// mounted (read-only) inside the sandbox. Under /run so it sits on the same
	// private tmpfs as the control sockets and needs no base-rootfs mountpoint.
	initTargetPath = "/run/n9e-sandbox-init"

	// controlTmpfs is the writable mount the control sockets + init binary are
	// bound onto; the base rootfs is read-only so a tmpfs is required to host the
	// bind targets (mirrors the --tmpfs /tmp the engine already mounts).
	controlTmpfs = "/run"
)
