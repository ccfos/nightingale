package sandbox

import (
	"net"
	"strings"
)

// egress_security.go is the OS-agnostic, pure policy core of the egress proxy
// (§10.4): the domain allowlist matcher and the SSRF deny classifier. It has no
// I/O so it is exhaustively unit-testable; egress_proxy.go calls it on every
// CONNECT/forward to decide whether a target host+IP may be dialled.

// normalizeHost lowercases a host and strips a single trailing dot (the FQDN
// root, which resolvers accept but allowlists never carry). Brackets around an
// IPv6 literal are removed by the caller's SplitHostPort, not here.
func normalizeHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

// hostAllowed reports whether host matches the allowlist. Two forms are honored:
//   - exact: "api.openai.com" matches only that host.
//   - wildcard label: "*.example.com" matches "a.example.com" and
//     "a.b.example.com" but NOT the apex "example.com" (add it explicitly).
//
// The design (§10.4) recommends specific hosts over broad wildcards to keep
// domain-fronting residual risk negligible; wildcards are supported but the
// operator owns that trade-off. Matching is done on the already-normalized host.
func hostAllowed(allowlist []string, host string) bool {
	host = normalizeHost(host)
	if host == "" {
		return false
	}
	for _, pat := range allowlist {
		pat = normalizeHost(pat)
		if pat == "" {
			continue
		}
		if suffix, ok := strings.CutPrefix(pat, "*."); ok {
			// "*.example.com" → host must end with ".example.com" and have at
			// least one label in front (so the apex itself does not match).
			if strings.HasSuffix(host, "."+suffix) && len(host) > len(suffix)+1 {
				return true
			}
			continue
		}
		if host == pat {
			return true
		}
	}
	return false
}

// parseDenyCIDRs turns the configured deny CIDR strings into *net.IPNet,
// silently skipping malformed entries (validated once at proxy construction; a
// bad CIDR must not crash a running proxy). It is additive on top of the
// built-in SSRF classes in ipDenied.
func parseDenyCIDRs(cidrs []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if _, n, err := net.ParseCIDR(c); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// ipDenied classifies a *resolved* IP against the SSRF denylist (§10.4): the
// built-in dangerous classes plus any operator deny CIDRs. It is checked AFTER
// DNS resolution and BEFORE dial, so DNS-rebinding to an internal address (low
// TTL flipping to 169.254.169.254 / RFC1918) is caught — the IP that is checked
// is the IP that is dialled (DNS pinning, enforced by the caller).
//
// Covered without needing every literal CIDR, via net.IP's own predicates:
//   - loopback         127.0.0.0/8, ::1
//   - unspecified      0.0.0.0, ::
//   - link-local       169.254.0.0/16 (incl. the IMDS 169.254.169.254), fe80::/10
//   - multicast/bcast  224.0.0.0/4, ff00::/8
//   - private          10/8, 172.16/12, 192.168/16, and ULA fc00::/7 (IsPrivate)
//
// IPv4-mapped IPv6 (::ffff:127.0.0.1, ::ffff:a9fe:a9fe) is normalized by net.IP
// to its v4 form by these predicates, so the mapped-address bypass is closed.
//
// loopback / unspecified / link-local / multicast are ALWAYS denied (there is no
// safe egress target there). The RFC1918+ULA private class is gated by
// denyPrivate (default true) so an on-prem operator with an internal allowlist
// can opt to permit it — a deliberate, configured choice (§16.3 DenyPrivateCIDRs).
func ipDenied(ip net.IP, extra []*net.IPNet, denyPrivate bool) (bool, string) {
	if ip == nil {
		return true, "unparseable ip"
	}
	switch {
	case ip.IsLoopback():
		return true, "loopback"
	case ip.IsUnspecified():
		return true, "unspecified"
	case ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast():
		return true, "link-local (incl. cloud metadata 169.254.169.254)"
	case ip.IsMulticast():
		return true, "multicast"
	case denyPrivate && ip.IsPrivate():
		return true, "private (RFC1918 / ULA)"
	}
	for _, n := range extra {
		if n.Contains(ip) {
			return true, "deny-cidr " + n.String()
		}
	}
	return false, ""
}

// literalIP returns the parsed IP if host is an IP literal, else nil. Hosts that
// are all-numeric but not a valid dotted/colon IP (decimal/octal/hex shorthand
// like "2130706433" or "0x7f.1") parse as nil here, so they are treated as
// names: they will not match a hostname allowlist (→ denied), and if they were
// to resolve, the resolved IP is still SSRF-checked. This closes the异形-IP
// bypass (§10.4) without a bespoke numeric parser.
func literalIP(host string) net.IP {
	return net.ParseIP(strings.Trim(host, "[]"))
}
