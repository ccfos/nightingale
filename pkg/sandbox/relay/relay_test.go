package relay

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestServeBridgesTCPToBackend verifies the forwarder splices a loopback TCP
// client to the backend (here a UDS echo) bidirectionally — the §10.2 data path
// minus the real netns.
func TestServeBridgesTCPToBackend(t *testing.T) {
	// Backend: a UDS echo server (stands in for the host egress proxy). Use a
	// short /tmp path — macOS's default TMPDIR exceeds the 108-byte sun_path cap.
	dir, err := os.MkdirTemp("/tmp", "n9erelay")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	defer os.RemoveAll(dir)
	uds := filepath.Join(dir, "backend.sock")
	bl, err := net.Listen("unix", uds)
	if err != nil {
		t.Fatalf("backend listen: %v", err)
	}
	defer bl.Close()
	go func() {
		for {
			c, aerr := bl.Accept()
			if aerr != nil {
				return
			}
			go func() { _, _ = io.Copy(c, c); c.Close() }()
		}
	}()

	// Forwarder: loopback TCP listener relayed to the UDS backend.
	fl, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("forwarder listen: %v", err)
	}
	defer fl.Close()
	go func() {
		_ = Serve(fl, func() (net.Conn, error) { return net.Dial("unix", uds) })
	}()

	c, err := net.Dial("tcp", fl.Addr().String())
	if err != nil {
		t.Fatalf("client dial: %v", err)
	}
	defer c.Close()

	if _, err := io.WriteString(c, "hello-relay"); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len("hello-relay"))
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(c, buf); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(buf) != "hello-relay" {
		t.Fatalf("echo = %q, want hello-relay", buf)
	}
}
