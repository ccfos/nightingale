// Package relay is the tiny TCP↔UDS bridge that runs INSIDE the sandbox network
// namespace as the egress forwarder (design §10.2). The script's HTTP_PROXY
// points at a loopback TCP address; stock HTTP clients (requests/urllib3/curl/
// wget/Go net/http) speak TCP, not UNIX sockets, so a relay must bridge that
// loopback TCP to the bind-mounted host UDS that reaches the egress proxy.
//
// Claude Code uses `socat` for this role; n9e ships it as Go instead, embedded
// in the n9e-sandbox-init binary (cmd/n9e-sandbox-init) so the host needs no
// external socat (§10.2 note). This package is deliberately a dependency-free
// leaf so that binary stays small and statically linkable, and so the bridge is
// unit-testable on any OS (a UDS endpoint stands in for the egress proxy).
package relay

import (
	"io"
	"net"
	"sync"
)

// Serve accepts loopback TCP connections on l and, for each, dials a fresh
// backend via dialBackend and splices the two together bidirectionally until
// either side closes. It blocks until l fails (e.g. is Closed) and returns that
// error. Per-connection failures are isolated — one bad dial never stops the
// accept loop. dialBackend is typically a UDS dialer to the egress proxy.
func Serve(l net.Listener, dialBackend func() (net.Conn, error)) error {
	for {
		c, err := l.Accept()
		if err != nil {
			return err
		}
		go handle(c, dialBackend)
	}
}

func handle(client net.Conn, dialBackend func() (net.Conn, error)) {
	defer client.Close()
	backend, err := dialBackend()
	if err != nil {
		return
	}
	defer backend.Close()
	splice(client, backend)
}

// splice copies a↔b until both directions finish, half-closing each side as its
// source drains so an HTTP CONNECT tunnel's EOF propagates correctly.
func splice(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go copyHalf(a, b, &wg)
	go copyHalf(b, a, &wg)
	wg.Wait()
}

// copyHalf streams src→dst then half-closes dst's write side (CloseWrite) when
// supported, so the peer sees EOF without tearing down the read side still in
// use by the other half.
func copyHalf(dst, src net.Conn, wg *sync.WaitGroup) {
	defer wg.Done()
	_, _ = io.Copy(dst, src)
	if cw, ok := dst.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
}
