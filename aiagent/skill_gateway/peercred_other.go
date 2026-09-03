//go:build !linux

package skillgateway

import "net"

// verifyPeer is a no-op off Linux: SO_PEERCRED is Linux-specific, and the gateway
// is only wired for the bubblewrap (Linux) engine anyway. Kept so the package
// builds on the developer's host for tests.
func verifyPeer(net.Conn) error { return nil }
