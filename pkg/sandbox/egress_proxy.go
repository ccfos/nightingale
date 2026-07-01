package sandbox

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/toolkits/pkg/logger"
)

// egress_proxy.go is the host-side, out-of-sandbox egress proxy (§10.2/§10.4):
// the single point where every outbound connection from a network=proxy sandbox
// is checked. It listens on a per-exec UNIX socket (bind-mounted into exactly one
// sandbox), speaks the stock HTTP forward-proxy protocol so unmodified
// requests/urllib3/curl/wget/Go-net-http "just work", and enforces:
//
//   - domain allowlist (specific hosts; wildcards allowed but discouraged);
//   - DNS pinning — resolve once, SSRF-check the resolved IP, then dial THAT IP,
//     so a low-TTL rebind to an internal address cannot slip through;
//   - SSRF denylist on the resolved IP (loopback/RFC1918/link-local+IMDS/ULA/
//     IPv4-mapped/multicast + operator deny CIDRs);
//   - TCP only (UDP/QUIC cannot traverse a CONNECT/forward proxy → HTTP/3 domain-
//     filter bypass is structurally impossible here, §10.4);
//   - audit of host/port/ip/verdict/bytes/duration.
//
// TLS is NOT decrypted (§10.3): for HTTPS the client sends CONNECT and the proxy
// tunnels bytes after the allowlist+IP checks — IP-layer protection intact, no CA
// to install, end-to-end TLS preserved. The cost (cannot inject external creds on
// the wire) is accepted for phase 1; n9e-internal creds never come this way (they
// go through the Skill Gateway UDS, §12).
//
// VERIFICATION STATUS: the proxy + policy core are pure host-side Go, exercised
// by egress_proxy_test.go / egress_security_test.go. The end-to-end path through
// a real netns + the n9e-sandbox-init forwarder needs a Linux host to validate
// (design §18.2), like the rest of the bubblewrap engine.

// EgressAudit is one decision record emitted per outbound attempt (§10.4 audit).
type EgressAudit struct {
	ExecID    string
	Method    string // CONNECT | GET | POST | ...
	Host      string
	Port      string
	PinnedIP  string // the IP actually dialled (DNS-pinned), "" when denied early
	Allowed   bool
	Reason    string // deny reason when !Allowed
	BytesUp   int64
	BytesDown int64
	Duration  time.Duration
}

// EgressOptions configures one EgressProxy. The allowlist + deny CIDRs come from
// the global skill policy envelope; the callbacks let the control plane wire in
// per-session audit + the JIT "new domain" confirmation (§10.4 / §11.2).
type EgressOptions struct {
	ExecID         string
	Allowlist      []string // skill_policy.egress_allowlist (specific hosts)
	DenyCIDRs      []string // hard-deny CIDRs on top of the built-in SSRF classes
	DenyPrivate    bool     // deny RFC1918/ULA (default true; see ipDenied)
	AllowPlainHTTP bool     // forward absolute-form plain-HTTP (else CONNECT-only)
	DialTimeout    time.Duration
	IdleTimeout    time.Duration

	// OnNewDomain is consulted for a host NOT on the allowlist: return true to
	// allow this once (JIT confirmation in the chat), false/nil to deny (managed
	// lockdown). Phase 1 leaves it nil → deny, matching Claude Code's
	// allowManagedDomainsOnly (§10.4).
	OnNewDomain func(host string) bool
	// OnAudit receives every decision; nil falls back to a debug log line.
	OnAudit func(EgressAudit)
}

// EgressProxy is a running per-exec egress proxy. Build it with StartEgressProxy
// and stop it with Close when the execution ends.
type EgressProxy struct {
	opts        EgressOptions
	denyCIDRs   []*net.IPNet
	dialTimeout time.Duration
	idleTimeout time.Duration
	connSem     chan struct{} // per-exec concurrent-connection cap (bounds host goroutines/fds)

	// lookupIP / dialTCP are seams: production wires the host resolver + a plain
	// TCP dial, tests inject deterministic ones (no real DNS / public IP needed).
	lookupIP func(host string) ([]net.IP, error)
	dialTCP  func(addr string) (net.Conn, error)

	ln     net.Listener
	path   string
	closed atomic.Bool
}

const (
	defaultEgressDialTimeout = 10 * time.Second
	defaultEgressIdleTimeout = 120 * time.Second
)

// Bounds on the host-side request parser and accept loop. The peer is the
// untrusted in-sandbox process, so these cap what it can make the host spend
// before a request is even authorized (parallel to net/http's ReadHeaderTimeout
// / MaxHeaderBytes, which this hand-rolled CONNECT parser doesn't get for free).
const (
	egressHeaderReadTimeout = 10 * time.Second // slowloris cutoff for the parse phase
	maxRequestLineLen       = 8 << 10          // CONNECT target / absolute-form URI
	maxHeaderLineLen        = 8 << 10          // per header line
	maxHeaderBytesTotal     = 64 << 10         // cumulative header bytes
	maxHeaderCount          = 100
	maxEgressConns          = 64 // concurrent connections per execution
)

// StartEgressProxy binds udsPath, starts the accept loop in the background, and
// returns the running proxy. The socket is created 0600 (host-private; it
// reaches the sandbox only via the engine's bind-mount, never via the
// filesystem). Caller must Close() it when the run finishes.
func StartEgressProxy(udsPath string, opts EgressOptions) (*EgressProxy, error) {
	p := newEgressProxy(opts)
	// A stale socket from a crashed run would make Listen fail with EADDRINUSE.
	_ = os.Remove(udsPath)
	ln, err := net.Listen("unix", udsPath)
	if err != nil {
		return nil, fmt.Errorf("egress proxy listen %s: %w", udsPath, err)
	}
	_ = os.Chmod(udsPath, 0o600)
	p.ln = ln
	p.path = udsPath
	go p.acceptLoop()
	return p, nil
}

// newEgressProxy builds an unbound proxy with production seams. Split out so the
// listener-less policy/protocol core is constructible in tests.
func newEgressProxy(opts EgressOptions) *EgressProxy {
	dialTimeout := orDur(opts.DialTimeout, defaultEgressDialTimeout)
	resolver := &net.Resolver{} // host resolver; the sandbox has no DNS of its own
	return &EgressProxy{
		opts:        opts,
		denyCIDRs:   parseDenyCIDRs(opts.DenyCIDRs),
		dialTimeout: dialTimeout,
		idleTimeout: orDur(opts.IdleTimeout, defaultEgressIdleTimeout),
		connSem:     make(chan struct{}, maxEgressConns),
		lookupIP: func(host string) ([]net.IP, error) {
			ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
			defer cancel()
			return resolver.LookupIP(ctx, "ip", host)
		},
		dialTCP: func(addr string) (net.Conn, error) {
			return net.DialTimeout("tcp", addr, dialTimeout)
		},
	}
}

// SocketPath is the host UDS path the proxy listens on (for the bind-mount).
func (p *EgressProxy) SocketPath() string { return p.path }

// Close stops accepting and removes the socket. In-flight tunnels end when the
// sandbox (and thus its forwarder) tears down — the proxy connections are bound
// by the sandbox lifetime, so we don't force-kill them here.
func (p *EgressProxy) Close() {
	if p == nil || !p.closed.CompareAndSwap(false, true) {
		return
	}
	if p.ln != nil {
		_ = p.ln.Close()
	}
	_ = os.Remove(p.path)
}

func (p *EgressProxy) acceptLoop() {
	for {
		c, err := p.ln.Accept()
		if err != nil {
			if p.closed.Load() {
				return
			}
			logger.Warningf("sandbox egress[%s]: accept: %v", p.opts.ExecID, err)
			return
		}
		// Per-exec connection cap. Blocking acquire backpressures the accept loop
		// when the sandbox opens more than maxEgressConns connections; the parse
		// read deadline guarantees a stuck connection releases its slot, so this
		// can't wedge. Bounds host goroutines/fds against a connection flood.
		p.connSem <- struct{}{}
		go func() {
			defer func() { <-p.connSem }()
			p.handleConn(c)
		}()
	}
}

func (p *EgressProxy) handleConn(client net.Conn) {
	defer client.Close()
	// The sandbox controls every byte on this socket, so bound the request-parse
	// phase: a read deadline cuts a slow/idle sender (slowloris) and the
	// readRequestLine/readHeaders byte caps stop an unbounded line from growing
	// host memory. splice() clears this deadline when it hands off to the tunnel,
	// which then applies its own idle timeout.
	_ = client.SetReadDeadline(time.Now().Add(egressHeaderReadTimeout))
	br := bufio.NewReader(client)

	method, target, ok := readRequestLine(br)
	if !ok {
		return
	}
	if strings.EqualFold(method, "CONNECT") {
		p.handleConnect(client, br, target)
		return
	}
	if p.opts.AllowPlainHTTP {
		p.handlePlain(client, br, method, target)
		return
	}
	writeStatus(client, "405 Method Not Allowed")
	p.audit(EgressAudit{Method: method, Host: target, Reason: "plain-HTTP forwarding disabled (HTTPS/CONNECT only)"})
}

// handleConnect tunnels an HTTPS CONNECT after the host+IP checks (§10.3). The
// proxy never sees plaintext: it only learns the SNI-equivalent host from the
// CONNECT line, which is exactly what the allowlist needs.
func (p *EgressProxy) handleConnect(client net.Conn, br *bufio.Reader, target string) {
	if err := drainHeaders(br); err != nil {
		return
	}
	host, port := splitHostPortDefault(target, "443")
	start := time.Now()

	upstream, ip, reason, err := p.authorizeAndDial(host, port)
	if err != nil {
		writeStatus(client, "403 Forbidden")
		p.audit(EgressAudit{Method: "CONNECT", Host: host, Port: port, Reason: reason, Duration: time.Since(start)})
		return
	}
	defer upstream.Close()

	if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	up, down := p.splice(client, br, upstream)
	p.audit(EgressAudit{
		Method: "CONNECT", Host: host, Port: port, PinnedIP: ip, Allowed: true,
		BytesUp: up, BytesDown: down, Duration: time.Since(start),
	})
}

// handlePlain forwards an absolute-form plain-HTTP request ("GET http://h/p ...")
// after the same host+IP checks, rewriting it to origin form for the upstream and
// forcing Connection: close (one request per connection — skills almost never
// pipeline plain HTTP, and HTTPS is the dominant path anyway).
func (p *EgressProxy) handlePlain(client net.Conn, br *bufio.Reader, method, target string) {
	start := time.Now()
	host, port, originPath, perr := parseAbsoluteURI(target)
	if perr != nil {
		writeStatus(client, "400 Bad Request")
		p.audit(EgressAudit{Method: method, Host: target, Reason: "bad absolute-form request URI"})
		return
	}
	headers, err := readHeaders(br)
	if err != nil {
		return
	}

	upstream, ip, reason, derr := p.authorizeAndDial(host, port)
	if derr != nil {
		writeStatus(client, "403 Forbidden")
		p.audit(EgressAudit{Method: method, Host: host, Port: port, Reason: reason, Duration: time.Since(start)})
		return
	}
	defer upstream.Close()

	// Replay the request to the upstream in origin form with hop-by-hop/proxy
	// headers stripped and keep-alive disabled.
	var reqb strings.Builder
	fmt.Fprintf(&reqb, "%s %s HTTP/1.1\r\n", method, originPath)
	for _, h := range sanitizeRequestHeaders(headers) {
		reqb.WriteString(h)
		reqb.WriteString("\r\n")
	}
	reqb.WriteString("Connection: close\r\n\r\n")
	if _, err := io.WriteString(upstream, reqb.String()); err != nil {
		return
	}

	up, down := p.splice(client, br, upstream)
	p.audit(EgressAudit{
		Method: method, Host: host, Port: port, PinnedIP: ip, Allowed: true,
		BytesUp: up + int64(reqb.Len()), BytesDown: down, Duration: time.Since(start),
	})
}

// authorizeAndDial is the policy choke point shared by CONNECT and plain HTTP:
// allowlist (or JIT) → resolve → SSRF-check → dial the pinned IP. The IP that is
// checked is the IP that is dialled (no second resolve), which is the whole point
// of DNS pinning (§10.4).
func (p *EgressProxy) authorizeAndDial(host, port string) (conn net.Conn, pinnedIP, reason string, err error) {
	host = normalizeHost(host)
	if host == "" {
		return nil, "", "empty host", fmt.Errorf("empty host")
	}
	if n, perr := strconv.Atoi(port); perr != nil || n < 1 || n > 65535 {
		return nil, "", "invalid port " + port, fmt.Errorf("invalid port")
	}

	if !hostAllowed(p.opts.Allowlist, host) {
		if p.opts.OnNewDomain == nil || !p.opts.OnNewDomain(host) {
			return nil, "", "host not in egress allowlist", fmt.Errorf("denied: %s", host)
		}
	}

	ips, rerr := p.resolveHost(host)
	if rerr != nil {
		return nil, "", "dns resolution failed: " + rerr.Error(), rerr
	}

	var lastReason string
	for _, ip := range ips {
		if denied, why := ipDenied(ip, p.denyCIDRs, p.opts.DenyPrivate); denied {
			lastReason = "SSRF: resolved IP " + ip.String() + " is " + why
			continue
		}
		// DNS pinning: dial the exact IP that just passed the SSRF check, never a
		// re-resolved name, so a low-TTL rebind cannot swap in an internal IP.
		c, derr := p.dialTCP(net.JoinHostPort(ip.String(), port))
		if derr != nil {
			lastReason = "dial " + ip.String() + ": " + derr.Error()
			continue
		}
		return c, ip.String(), "", nil
	}
	if lastReason == "" {
		lastReason = "no usable address"
	}
	return nil, "", lastReason, fmt.Errorf("no dialable ip for %s", host)
}

// resolveHost returns the candidate IPs for a host: the literal IP itself when
// host is an IP, else a single DNS resolution (the pinned set). It deliberately
// does NOT let the script influence resolution — the host resolver is used.
func (p *EgressProxy) resolveHost(host string) ([]net.IP, error) {
	if lit := literalIP(host); lit != nil {
		return []net.IP{lit}, nil
	}
	return p.lookupIP(host)
}

// splice runs the bidirectional copy with an idle timeout, returning the byte
// counts (client→upstream, upstream→client). The client read side is the
// bufio.Reader so any bytes already buffered past the request are not lost.
func (p *EgressProxy) splice(client net.Conn, clientR io.Reader, upstream net.Conn) (up, down int64) {
	// Drop the parse-phase read deadline; from here copyIdle owns the (idle)
	// deadlines so a long but active tunnel is never cut by the header timeout.
	_ = client.SetReadDeadline(time.Time{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		up = copyIdle(upstream, clientR, upstream, p.idleTimeout)
		halfClose(upstream)
	}()
	go func() {
		defer wg.Done()
		down = copyIdle(client, upstream, client, p.idleTimeout)
		halfClose(client)
	}()
	wg.Wait()
	return up, down
}

func (p *EgressProxy) audit(a EgressAudit) {
	a.ExecID = p.opts.ExecID
	if p.opts.OnAudit != nil {
		p.opts.OnAudit(a)
		return
	}
	if a.Allowed {
		logger.Debugf("sandbox egress[%s]: ALLOW %s %s:%s ip=%s up=%d down=%d dur=%s",
			a.ExecID, a.Method, a.Host, a.Port, a.PinnedIP, a.BytesUp, a.BytesDown, a.Duration)
	} else {
		logger.Infof("sandbox egress[%s]: DENY %s %s:%s — %s", a.ExecID, a.Method, a.Host, a.Port, a.Reason)
	}
}

// --- small HTTP/wire helpers (no net/http server; we need CONNECT hijack) ---

// copyIdle copies src→dst, refreshing an idle read deadline on idleConn each
// iteration so a stalled tunnel is reclaimed without capping long transfers.
func copyIdle(dst io.Writer, src io.Reader, idleConn net.Conn, idle time.Duration) int64 {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		if idle > 0 {
			_ = idleConn.SetReadDeadline(time.Now().Add(idle))
		}
		n, rerr := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return total
			}
			total += int64(n)
		}
		if rerr != nil {
			return total
		}
	}
}

func halfClose(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
}

// readLineLimited reads through the next '\n' and returns the line with any
// trailing CR/LF trimmed, failing if it grows past max bytes before the newline.
// Hand-rolled (vs ReadString) so an unbounded line from the untrusted sandbox
// can't balloon host memory. The bufio.Reader keeps any bytes already read past
// the line, so a later splice still sees the buffered body.
func readLineLimited(br *bufio.Reader, max int) (string, error) {
	var sb strings.Builder
	for {
		b, err := br.ReadByte()
		if err != nil {
			return "", err
		}
		if b == '\n' {
			return strings.TrimRight(sb.String(), "\r"), nil
		}
		if sb.Len() >= max {
			return "", fmt.Errorf("line exceeds %d bytes", max)
		}
		sb.WriteByte(b)
	}
}

// readRequestLine reads "METHOD TARGET HTTP/x.y" and returns (method, target).
func readRequestLine(br *bufio.Reader) (method, target string, ok bool) {
	line, err := readLineLimited(br, maxRequestLineLen)
	if err != nil {
		return "", "", false
	}
	f := strings.Fields(line)
	if len(f) < 2 {
		return "", "", false
	}
	return f[0], f[1], true
}

func drainHeaders(br *bufio.Reader) error {
	_, err := readHeaders(br)
	return err
}

func readHeaders(br *bufio.Reader) ([]string, error) {
	var hs []string
	total := 0
	for {
		line, err := readLineLimited(br, maxHeaderLineLen)
		if err != nil {
			return nil, err
		}
		if line == "" {
			return hs, nil
		}
		total += len(line)
		if total > maxHeaderBytesTotal {
			return nil, fmt.Errorf("headers exceed %d bytes", maxHeaderBytesTotal)
		}
		if len(hs) >= maxHeaderCount {
			return nil, fmt.Errorf("too many headers (>%d)", maxHeaderCount)
		}
		hs = append(hs, line)
	}
}

// sanitizeRequestHeaders drops hop-by-hop and proxy-bearing headers (§10.4
// credential/header hygiene) so the script's own proxy env / connection control
// is never forwarded upstream.
func sanitizeRequestHeaders(headers []string) []string {
	out := headers[:0]
	for _, h := range headers {
		name := strings.ToLower(strings.TrimSpace(strings.SplitN(h, ":", 2)[0]))
		switch name {
		case "connection", "proxy-connection", "proxy-authorization", "keep-alive",
			"transfer-encoding", "te", "trailer", "upgrade":
			continue
		}
		out = append(out, h)
	}
	return out
}

func writeStatus(c net.Conn, status string) {
	_, _ = io.WriteString(c, "HTTP/1.1 "+status+"\r\nConnection: close\r\n\r\n")
}

// splitHostPortDefault splits "host:port"; a bare host (or bracketed IPv6) gets
// the supplied default port.
func splitHostPortDefault(hostport, defPort string) (host, port string) {
	if h, p, err := net.SplitHostPort(hostport); err == nil {
		return h, p
	}
	return strings.Trim(hostport, "[]"), defPort
}

// parseAbsoluteURI parses an absolute-form request target ("http://host[:port]
// /path?q") into host, port (default 80) and the origin-form path. https://
// absolute form is rejected — TLS must use CONNECT, never plaintext forwarding.
func parseAbsoluteURI(target string) (host, port, originPath string, err error) {
	rest, ok := strings.CutPrefix(target, "http://")
	if !ok {
		return "", "", "", fmt.Errorf("not an http:// absolute-form URI")
	}
	slash := strings.IndexByte(rest, '/')
	authority := rest
	originPath = "/"
	if slash >= 0 {
		authority = rest[:slash]
		originPath = rest[slash:]
	}
	if authority == "" {
		return "", "", "", fmt.Errorf("missing authority")
	}
	host, port = splitHostPortDefault(authority, "80")
	return host, port, originPath, nil
}

func orDur(v, def time.Duration) time.Duration {
	if v <= 0 {
		return def
	}
	return v
}
