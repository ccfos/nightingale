package tools

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateFetchURL(t *testing.T) {
	cases := []struct {
		url     string
		wantErr bool
	}{
		{"https://example.com/foo", false},
		{"http://example.com", false},
		{"ftp://example.com", true},
		{"file:///etc/passwd", true},
		{"https://", true},
		{"://nope", true},
	}
	for _, c := range cases {
		err := validateFetchURL(c.url)
		if (err != nil) != c.wantErr {
			t.Errorf("validateFetchURL(%q) err=%v wantErr=%v", c.url, err, c.wantErr)
		}
	}
}

func TestIsPublicIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"1.1.1.1", true},
		{"8.8.8.8", true},
		{"127.0.0.1", false},      // loopback
		{"10.0.0.1", false},       // RFC1918
		{"192.168.1.1", false},    // RFC1918
		{"172.16.0.1", false},     // RFC1918
		{"169.254.0.1", false},    // link-local
		{"0.0.0.0", false},        // unspecified
		{"100.64.0.1", false},     // CGNAT
		{"192.0.2.1", false},      // TEST-NET-1
		{"198.51.100.1", false},   // TEST-NET-2
		{"203.0.113.1", false},    // TEST-NET-3
		{"198.18.0.1", false},     // benchmarking
		{"240.0.0.1", false},      // reserved
		{"::1", false},            // ipv6 loopback
		{"fe80::1", false},        // ipv6 link-local
		{"fc00::1", false},        // ipv6 ULA
		{"2606:4700:4700::1111", true}, // cloudflare ipv6
	}
	for _, c := range cases {
		got := isPublicIP(net.ParseIP(c.ip))
		if got != c.want {
			t.Errorf("isPublicIP(%s) = %v, want %v", c.ip, got, c.want)
		}
	}
}

// TestIsPublicIP_RejectsIPv6EmbeddedForbiddenRanges guards against a
// regression where isPublicIP only unwrapped the standard "::ffff:a.b.c.d"
// IPv4-mapped form via ip.To4() and never inspected 6to4 (2002::/16), NAT64
// (64:ff9b::/96, 64:ff9b:1::/48), or the deprecated IPv6 site-local range
// (fec0::/10) for an embedded/forbidden address such as the cloud
// instance-metadata endpoint, 169.254.169.254.
func TestIsPublicIP_RejectsIPv6EmbeddedForbiddenRanges(t *testing.T) {
	cases := []struct {
		name string
		ip   string
	}{
		// 2002:a9fe:a9fe:: is the 6to4 encoding of 169.254.169.254 (IMDS).
		{"6to4-IMDS", "2002:a9fe:a9fe::"},
		// 64:ff9b::a9fe:a9fe is the well-known NAT64 encoding of 169.254.169.254.
		{"NAT64-IMDS", "64:ff9b::a9fe:a9fe"},
		// 64:ff9b:1::a9fe:a9fe is the NAT64 local-use encoding of the same address.
		{"NAT64-local-IMDS", "64:ff9b:1::a9fe:a9fe"},
		// fec0::1 is deprecated IPv6 site-local, still routed on some networks.
		{"sitelocal-fec0", "fec0::1"},
		// 169.254.169.254 and its IPv4-mapped form were already correctly
		// rejected; kept here so a regression in the new code can't
		// accidentally start allowing these too.
		{"direct-IMDS", "169.254.169.254"},
		{"mapped-IMDS", "::ffff:169.254.169.254"},
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("net.ParseIP(%q) returned nil", c.ip)
		}
		if got := isPublicIP(ip); got {
			t.Errorf("isPublicIP(%s / %s) = true, want false", c.name, c.ip)
		}
	}

	// A 6to4 address that embeds a genuinely public IPv4 address must still
	// be allowed; the fix must not turn 2002::/16 into a blanket block.
	if ip := net.ParseIP("2002:0808:0808::"); !isPublicIP(ip) {
		t.Errorf("isPublicIP(6to4 embedding 8.8.8.8) = false, want true")
	}
}

// TestSafeDialContext_RejectsIPv6EmbeddedForbiddenRange drives the
// unmodified safeDialContext (the function net/http actually uses to open
// the TCP connection for http_fetch) with the 6to4 encoding of
// 169.254.169.254 and confirms it is rejected at the gate before any network
// dial is attempted, the same as the existing loopback/RFC1918/link-local
// controls.
func TestSafeDialContext_RejectsIPv6EmbeddedForbiddenRange(t *testing.T) {
	ctx := context.Background()

	for _, target := range []string{
		net.JoinHostPort("127.0.0.1", "9"),
		net.JoinHostPort("2002:a9fe:a9fe::", "9"),
	} {
		if _, err := safeDialContext(ctx, "tcp", target); err == nil || !strings.Contains(err.Error(), "non-public address") {
			t.Errorf("safeDialContext(%s) should be rejected at the gate, got err=%v", target, err)
		}
	}
}

func TestSweepStaleFetchTempFiles(t *testing.T) {
	dir := os.TempDir()

	// mkFetch creates an http_fetch-style temp file and ages it by ageHours.
	mkFetch := func(ageHours int) string {
		f, err := os.CreateTemp(dir, HTTPFetchTempFilePrefix+"*")
		if err != nil {
			t.Fatalf("create temp: %v", err)
		}
		name := f.Name()
		f.Close()
		mt := time.Now().Add(-time.Duration(ageHours) * time.Hour)
		if err := os.Chtimes(name, mt, mt); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
		return name
	}

	old1 := mkFetch(10) // older than the 6h TTL → reaped
	old2 := mkFetch(7)  // older than the 6h TTL → reaped
	fresh := mkFetch(1) // within TTL → kept
	t.Cleanup(func() { os.Remove(fresh) })

	// A temp file WITHOUT our prefix must never be touched, even when stale.
	otherF, err := os.CreateTemp(dir, "some-other-tool-*")
	if err != nil {
		t.Fatalf("create other temp: %v", err)
	}
	other := otherF.Name()
	otherF.Close()
	t.Cleanup(func() { os.Remove(other) })
	staleTime := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(other, staleTime, staleTime); err != nil {
		t.Fatalf("chtimes other: %v", err)
	}

	SweepStaleFetchTempFiles(fetchTempTTL)

	exists := func(p string) bool {
		_, err := os.Stat(p)
		return err == nil
	}
	if exists(old1) || exists(old2) {
		t.Errorf("stale fetch temp files not reaped: old1=%v old2=%v", exists(old1), exists(old2))
	}
	if !exists(fresh) {
		t.Errorf("fresh fetch temp file %s was wrongly reaped", filepath.Base(fresh))
	}
	if !exists(other) {
		t.Errorf("non-fetch temp file %s was wrongly reaped", filepath.Base(other))
	}
}
