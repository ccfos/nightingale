package tools

import (
	"net"
	"os"
	"path/filepath"
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
