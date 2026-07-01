package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestEgressEndToEndReal drives the WHOLE egress data path with real components
// and the real internet — no mocks:
//
//	curl (HTTP(S)_PROXY) → n9e-sandbox-init forwarder (loopback TCP → UDS)
//	  → EgressProxy (allowlist / DNS-pin / SSRF) → real external host
//
// It is the §10.2 path minus only the bubblewrap netns wrapper (which is an
// isolation boundary around this exact byte path, not part of it). Skipped unless
// N9E_EGRESS_E2E=1, since it needs outbound network, curl, and the forwarder
// binary (N9E_FORWARDER=/path, or it builds one into a temp dir).
func TestEgressEndToEndReal(t *testing.T) {
	if os.Getenv("N9E_EGRESS_E2E") == "" {
		t.Skip("set N9E_EGRESS_E2E=1 to run the real-network egress e2e (needs curl + outbound network)")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not found")
	}
	fwd := forwarderBinary(t)

	// A short /tmp dir keeps the UDS under the 108-byte sun_path limit.
	dir, err := os.MkdirTemp("/tmp", "n9ee2e")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	sock := filepath.Join(dir, "egress.sock")

	// Allowlist example.com (a stable public 200) AND 127.0.0.1 — the latter to
	// prove SSRF beats the allowlist: an explicitly allowed host that resolves to
	// a forbidden IP is STILL blocked.
	var audits []EgressAudit
	p, err := StartEgressProxy(sock, EgressOptions{
		ExecID:         "se_e2e",
		Allowlist:      []string{"example.com", "127.0.0.1"},
		DenyPrivate:    true,
		AllowPlainHTTP: true,
		DialTimeout:    10 * time.Second,
		OnAudit:        func(a EgressAudit) { audits = append(audits, a) },
	})
	if err != nil {
		t.Fatalf("start egress proxy: %v", err)
	}
	defer p.Close()

	// run launches: forwarder --forward LISTEN=sock -- curl <url>, with HTTP(S)_PROXY
	// pointed at the forwarder (exactly how the engine wires a network=proxy skill).
	run := func(url string) (int, string) {
		t.Helper()
		cmd := exec.Command(fwd,
			"--forward", EgressForwarderListen+"="+sock,
			"--", "curl", "-sS", "-o", "/dev/null", "-w", "%{http_code}", "--max-time", "20", url)
		proxy := "http://" + EgressForwarderListen
		// Match the production injection (control.go): both cases, so any client
		// picks it up. (Apple's macOS curl ignores these and uses system config —
		// run this on Linux, where curl honours them.)
		cmd.Env = append(os.Environ(),
			"HTTP_PROXY="+proxy, "HTTPS_PROXY="+proxy,
			"http_proxy="+proxy, "https_proxy="+proxy)
		out, _ := cmd.CombinedOutput()
		code := 0
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
		return code, strings.TrimSpace(string(out))
	}

	// Happy path: a real HTTPS request to the allowlisted host returns a real 200
	// through the CONNECT tunnel (the proxy never sees plaintext).
	if code, out := run("https://example.com"); code != 0 || out != "200" {
		t.Fatalf("allowlisted https://example.com should return real 200, got exit=%d out=%q", code, out)
	}
	t.Log("https://example.com → 200 (real TLS tunnel through the egress proxy)")

	// Denied paths. We assert on the AUDIT decision (the authoritative security
	// signal), not curl's exit code: an HTTPS CONNECT denial fails curl, but a
	// plain-HTTP denial is delivered as a 403 response (curl exits 0 with code
	// 403) — in both cases the request never reached the target.
	run("https://www.google.com") // not in allowlist
	run("http://127.0.0.1:80")    // allowlisted host, but SSRF-forbidden IP

	got := map[string]EgressAudit{}
	for _, a := range audits {
		got[a.Host] = a
		t.Logf("audit: host=%s allowed=%v ip=%s reason=%q", a.Host, a.Allowed, a.PinnedIP, a.Reason)
	}

	if a, ok := got["example.com"]; !ok || !a.Allowed || a.PinnedIP == "" {
		t.Fatalf("example.com must be allowed with a pinned IP, got %+v", a)
	}
	if a, ok := got["www.google.com"]; !ok || a.Allowed || !strings.Contains(a.Reason, "allowlist") {
		t.Fatalf("www.google.com must be denied (not in allowlist), got %+v", a)
	}
	if a, ok := got["127.0.0.1"]; !ok || a.Allowed || !strings.Contains(a.Reason, "SSRF") {
		t.Fatalf("127.0.0.1 must be SSRF-denied despite being allowlisted, got %+v", a)
	}
}

// TestEgressThroughBwrapNetns is the FULL §10.2 topology on a real Linux host: a
// bubblewrap sandbox with its own loopback-only network namespace, the forwarder
// running INSIDE that netns, reaching the host egress proxy across the netns
// boundary through the bind-mounted UDS. This is the one path the unit tests
// can't reach — it proves netns isolation + cross-netns UDS + the launcher all
// work together, not just the byte relay.
//
// Skipped unless N9E_EGRESS_E2E=1 with bwrap + curl present, and self-skips if
// the environment forbids unprivileged user namespaces (e.g. a locked-down
// container) rather than failing.
func TestEgressThroughBwrapNetns(t *testing.T) {
	if os.Getenv("N9E_EGRESS_E2E") == "" {
		t.Skip("set N9E_EGRESS_E2E=1 to run the real bwrap-netns egress e2e")
	}
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		t.Skip("bwrap not installed")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not found (needed as the in-sandbox client)")
	}
	fwd := forwarderBinary(t)

	dir, err := os.MkdirTemp("/tmp", "n9ebw")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	sock := filepath.Join(dir, "egress.sock")

	var audits []EgressAudit
	p, err := StartEgressProxy(sock, EgressOptions{
		ExecID:      "se_bwrap",
		Allowlist:   []string{"example.com"},
		DenyPrivate: true,
		DialTimeout: 10 * time.Second,
		OnAudit:     func(a EgressAudit) { audits = append(audits, a) },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	proxy := "http://" + EgressForwarderListen
	// Mirror the bubblewrap engine's network=proxy argv (engine_bwrap_linux.go):
	// private netns, /run tmpfs, forwarder + egress socket bound, launcher prefix.
	args := []string{
		"--unshare-net", "--unshare-pid", "--unshare-ipc",
		"--ro-bind", "/", "/", // use the host rootfs (has curl + CA certs)
		"--proc", "/proc", "--dev", "/dev", "--tmpfs", "/run",
		"--ro-bind", fwd, initTargetPath,
		"--bind", sock, EgressSocketTarget,
		"--setenv", "HTTP_PROXY", proxy, "--setenv", "HTTPS_PROXY", proxy,
		"--setenv", "http_proxy", proxy, "--setenv", "https_proxy", proxy,
		"--",
		initTargetPath, "--forward", EgressForwarderListen + "=" + EgressSocketTarget, "--",
		"curl", "-sS", "-o", "/dev/null", "-w", "%{http_code}", "--max-time", "20", "https://example.com",
	}
	out, runErr := exec.Command(bwrap, args...).CombinedOutput()
	low := strings.ToLower(string(out))
	if runErr != nil && (strings.Contains(low, "namespace") || strings.Contains(low, "permission") || strings.Contains(low, "clone")) {
		t.Skipf("bwrap cannot create namespaces in this environment (expected in locked-down containers): %v\n%s", runErr, out)
	}
	if !strings.Contains(string(out), "200") {
		t.Fatalf("expected 200 from inside the bwrap netns, got err=%v out=%q", runErr, out)
	}
	t.Logf("inside bwrap --unshare-net: curl https://example.com → %s", strings.TrimSpace(string(out)))

	if len(audits) == 0 || !audits[0].Allowed || audits[0].Host != "example.com" || audits[0].PinnedIP == "" {
		t.Fatalf("egress proxy did not see the request from inside the netns: %+v", audits)
	}
	t.Logf("proxy saw the cross-netns request: host=%s ip=%s (DNS-pinned)", audits[0].Host, audits[0].PinnedIP)
}

// TestNetworkNoneHasNoEgress proves the inverse: a bwrap sandbox with
// --unshare-net and NO forwarder/proxy has a loopback-only netns and therefore
// no egress at all — curl to a public host must fail. This is what network=none
// gives, and it shows the proxy is the ONLY way out (§10.1).
func TestNetworkNoneHasNoEgress(t *testing.T) {
	if os.Getenv("N9E_EGRESS_E2E") == "" {
		t.Skip("set N9E_EGRESS_E2E=1")
	}
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		t.Skip("bwrap not installed")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not found")
	}
	args := []string{
		"--unshare-net", "--unshare-pid",
		"--ro-bind", "/", "/", "--proc", "/proc", "--dev", "/dev",
		// Strip any inherited proxy env so this is a clean DIRECT attempt that can
		// only fail because the netns has no egress (not because a proxy var is set).
		"--unsetenv", "HTTP_PROXY", "--unsetenv", "HTTPS_PROXY",
		"--unsetenv", "http_proxy", "--unsetenv", "https_proxy",
		"--", "curl", "-sS", "-o", "/dev/null", "-w", "%{http_code}", "--max-time", "10", "https://example.com",
	}
	out, err := exec.Command(bwrap, args...).CombinedOutput()
	low := strings.ToLower(string(out))
	if err != nil && (strings.Contains(low, "namespace") || strings.Contains(low, "clone")) {
		t.Skipf("bwrap cannot create namespaces here: %v\n%s", err, out)
	}
	if err == nil && strings.Contains(string(out), "200") {
		t.Fatalf("network=none must have NO egress, but curl reached example.com: %q", out)
	}
	t.Logf("network=none correctly has no egress: curl failed inside the isolated netns (%s)", strings.TrimSpace(string(out)))
}

// forwarderBinary returns a runnable n9e-sandbox-init: the prebuilt one named by
// N9E_FORWARDER, else it builds one into a temp dir for the current GOOS/GOARCH.
func forwarderBinary(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("N9E_FORWARDER"); p != "" {
		return p
	}
	out := filepath.Join(t.TempDir(), "n9e-sandbox-init")
	cmd := exec.Command("go", "build", "-o", out, "github.com/ccfos/nightingale/v6/cmd/n9e-sandbox-init")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build forwarder: %v\n%s", err, b)
	}
	return out
}
