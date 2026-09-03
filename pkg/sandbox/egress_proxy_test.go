package sandbox

import (
	"bufio"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// shortSock returns a UNIX socket path under /tmp short enough for the 108-byte
// sun_path limit (the macOS default TMPDIR blows past it). The same limit caps
// where production control sockets may live — keep DataDir short.
func shortSock(t *testing.T, name string) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "n9esb")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, name)
}

// startTestProxy builds a seam-injected proxy bound to a temp UDS, returning the
// proxy and a func that dials a fresh client connection to it.
func startTestProxy(t *testing.T, opts EgressOptions, lookup func(string) ([]net.IP, error), dial func(string) (net.Conn, error)) (*EgressProxy, func() net.Conn) {
	t.Helper()
	p := newEgressProxy(opts)
	if lookup != nil {
		p.lookupIP = lookup
	}
	if dial != nil {
		p.dialTCP = dial
	}
	sock := shortSock(t, "egress.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	p.ln = ln
	p.path = sock
	go p.acceptLoop()
	t.Cleanup(p.Close)
	return p, func() net.Conn {
		c, derr := net.Dial("unix", sock)
		if derr != nil {
			t.Fatalf("dial proxy: %v", derr)
		}
		return c
	}
}

// echoServer is a TCP server that echoes bytes back — stands in for the upstream
// behind a CONNECT tunnel.
func echoServer(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("echo listen: %v", err)
	}
	go func() {
		for {
			c, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			go func() { _, _ = io.Copy(c, c); c.Close() }()
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return ln
}

func publicLookup(net.IP) func(string) ([]net.IP, error) {
	return func(string) ([]net.IP, error) { return []net.IP{net.ParseIP("93.184.216.34")}, nil }
}

func readStatusLine(t *testing.T, br *bufio.Reader) string {
	t.Helper()
	line, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	return strings.TrimRight(line, "\r\n")
}

func TestEgressConnectAllowAndTunnel(t *testing.T) {
	up := echoServer(t)
	auditCh := make(chan EgressAudit, 4)
	opts := EgressOptions{
		ExecID:      "se_test",
		Allowlist:   []string{"example.com"},
		DenyPrivate: true,
		OnAudit:     func(a EgressAudit) { auditCh <- a },
	}
	// Resolve to a public IP (passes SSRF), but dial the local echo server.
	_, dialClient := startTestProxy(t, opts,
		publicLookup(nil),
		func(string) (net.Conn, error) { return net.Dial("tcp", up.Addr().String()) },
	)

	c := dialClient()
	if _, err := io.WriteString(c, "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(c)
	if status := readStatusLine(t, br); !strings.Contains(status, "200") {
		t.Fatalf("CONNECT status = %q, want 200", status)
	}
	// drain the blank line after the status
	_, _ = br.ReadString('\n')

	// Tunnel is open: write through it and read the echo.
	if _, err := io.WriteString(c, "ping"); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(br, buf); err != nil {
		t.Fatalf("tunnel read: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("tunnel echo = %q, want ping", buf)
	}

	// Closing the client drains the tunnel, which emits the completed audit
	// record (with byte counts, §10.4).
	c.Close()
	select {
	case a := <-auditCh:
		if !a.Allowed || a.PinnedIP != "93.184.216.34" || a.BytesUp == 0 || a.BytesDown == 0 {
			t.Fatalf("audit = %+v, want allowed CONNECT to pinned public IP with bytes", a)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no egress audit emitted")
	}
}

func TestEgressConnectDenyNotAllowlisted(t *testing.T) {
	dialed := false
	_, dialClient := startTestProxy(t, EgressOptions{Allowlist: []string{"good.com"}, DenyPrivate: true},
		publicLookup(nil),
		func(string) (net.Conn, error) { dialed = true; return nil, io.EOF },
	)
	c := dialClient()
	defer c.Close()
	io.WriteString(c, "CONNECT evil.com:443 HTTP/1.1\r\n\r\n")
	if status := readStatusLine(t, bufio.NewReader(c)); !strings.Contains(status, "403") {
		t.Fatalf("status = %q, want 403 for non-allowlisted host", status)
	}
	if dialed {
		t.Error("must not dial upstream for a denied host")
	}
}

func TestEgressConnectDenySSRF(t *testing.T) {
	dialed := false
	// Host IS allowlisted, but it resolves to loopback → SSRF must block the dial.
	_, dialClient := startTestProxy(t, EgressOptions{Allowlist: []string{"internal.test"}, DenyPrivate: true},
		func(string) ([]net.IP, error) { return []net.IP{net.ParseIP("127.0.0.1")}, nil },
		func(string) (net.Conn, error) { dialed = true; return nil, io.EOF },
	)
	c := dialClient()
	defer c.Close()
	io.WriteString(c, "CONNECT internal.test:443 HTTP/1.1\r\n\r\n")
	if status := readStatusLine(t, bufio.NewReader(c)); !strings.Contains(status, "403") {
		t.Fatalf("status = %q, want 403 for SSRF-blocked IP", status)
	}
	if dialed {
		t.Error("DNS-pinned SSRF check must run BEFORE dial; upstream must not be dialled")
	}
}

func TestEgressPlainHTTPForward(t *testing.T) {
	// Upstream that returns a canned response and verifies it got origin form.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	gotReqLine := make(chan string, 1)
	go func() {
		c, aerr := ln.Accept()
		if aerr != nil {
			return
		}
		defer c.Close()
		line, _ := bufio.NewReader(c).ReadString('\n')
		gotReqLine <- strings.TrimRight(line, "\r\n")
		io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nhi")
	}()

	_, dialClient := startTestProxy(t, EgressOptions{Allowlist: []string{"example.com"}, DenyPrivate: true, AllowPlainHTTP: true},
		publicLookup(nil),
		func(string) (net.Conn, error) { return net.Dial("tcp", ln.Addr().String()) },
	)
	c := dialClient()
	defer c.Close()
	io.WriteString(c, "GET http://example.com/path?q=1 HTTP/1.1\r\nHost: example.com\r\nProxy-Connection: keep-alive\r\n\r\n")

	br := bufio.NewReader(c)
	if status := readStatusLine(t, br); !strings.Contains(status, "200") {
		t.Fatalf("status = %q, want 200", status)
	}
	select {
	case rl := <-gotReqLine:
		if rl != "GET /path?q=1 HTTP/1.1" {
			t.Fatalf("upstream got %q, want origin-form request line", rl)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("upstream never received the forwarded request")
	}
}

func TestEgressPlainHTTPDisabledByDefault(t *testing.T) {
	_, dialClient := startTestProxy(t, EgressOptions{Allowlist: []string{"example.com"}, DenyPrivate: true},
		publicLookup(nil), nil)
	c := dialClient()
	defer c.Close()
	io.WriteString(c, "GET http://example.com/ HTTP/1.1\r\n\r\n")
	if status := readStatusLine(t, bufio.NewReader(c)); !strings.Contains(status, "405") {
		t.Fatalf("status = %q, want 405 when AllowPlainHTTP is off", status)
	}
}
