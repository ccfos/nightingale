package tools

import (
	"net"
	"testing"
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
