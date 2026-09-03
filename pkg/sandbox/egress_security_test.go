package sandbox

import (
	"net"
	"testing"
)

func TestHostAllowed(t *testing.T) {
	allow := []string{"api.openai.com", "*.example.com", "EXACT.io"}
	cases := map[string]bool{
		"api.openai.com":   true,
		"API.OpenAI.com":   true,  // case-insensitive
		"api.openai.com.":  true,  // trailing FQDN dot tolerated
		"evil.com":         false, // not listed
		"a.example.com":    true,  // wildcard label
		"a.b.example.com":  true,  // multi-label under wildcard
		"example.com":      false, // apex is NOT matched by *.example.com
		"xexample.com":     false, // must be a real subdomain boundary
		"exact.io":         true,  // listed pattern normalized too
		"notexact.io":      false,
		"":                 false,
		"api.openai.com.x": false,
	}
	for host, want := range cases {
		if got := hostAllowed(allow, host); got != want {
			t.Errorf("hostAllowed(%q) = %v, want %v", host, got, want)
		}
	}
	if hostAllowed(nil, "anything.com") {
		t.Error("empty allowlist must deny all")
	}
	// bare "*" (the Egress="open" sentinel) allows any host.
	for _, h := range []string{"anything.com", "internal.corp", "1.2.3.4"} {
		if !hostAllowed([]string{"*"}, h) {
			t.Errorf("hostAllowed([*], %q) should allow", h)
		}
	}
}

func TestEgressPlan(t *testing.T) {
	cases := []struct {
		egress   string
		proxy    bool
		allowAll bool // allowlist == ["*"]
		denyPriv bool
	}{
		{"open", true, true, false},
		{"OPEN", true, true, false}, // case-insensitive
		{"allowlist", true, false, true},
		{"off", false, false, false},
		{"", true, true, false},          // unset → default open
		{"garbage", false, false, false}, // unrecognized → fail safe (off)
	}
	for _, c := range cases {
		cfg := Config{Egress: c.egress, SkillPolicy: SkillPolicyConfig{EgressAllowlist: []string{"a.com"}}}
		proxy, allow, denyPriv := cfg.EgressPlan()
		allowAll := len(allow) == 1 && allow[0] == "*"
		if proxy != c.proxy || denyPriv != c.denyPriv || allowAll != c.allowAll {
			t.Errorf("Egress=%q → proxy=%v allowAll=%v denyPriv=%v, want %v/%v/%v",
				c.egress, proxy, allowAll, denyPriv, c.proxy, c.allowAll, c.denyPriv)
		}
	}

	// open's catastrophic floor: with denyPrivate=false the IP gate must still let
	// RFC1918 through but ALWAYS block loopback + cloud metadata.
	_, _, denyPriv := Config{Egress: "open"}.EgressPlan()
	if d, _ := ipDenied(net.ParseIP("10.0.0.5"), nil, denyPriv); d {
		t.Error("open: RFC1918 (private) should be reachable")
	}
	if d, _ := ipDenied(net.ParseIP("127.0.0.1"), nil, denyPriv); !d {
		t.Error("open: host loopback must STILL be blocked")
	}
	if d, _ := ipDenied(net.ParseIP("169.254.169.254"), nil, denyPriv); !d {
		t.Error("open: cloud metadata must STILL be blocked")
	}
}

func TestIPDenied(t *testing.T) {
	extra := parseDenyCIDRs([]string{"203.0.113.0/24"}) // operator deny CIDR
	cases := []struct {
		ip          string
		denyPrivate bool
		want        bool
		note        string
	}{
		{"8.8.8.8", true, false, "public ok"},
		{"93.184.216.34", true, false, "public ok"},
		{"127.0.0.1", true, true, "loopback"},
		{"127.0.0.1", false, true, "loopback denied even with denyPrivate=false"},
		{"10.0.0.5", true, true, "RFC1918"},
		{"10.0.0.5", false, false, "RFC1918 allowed when denyPrivate=false"},
		{"192.168.1.1", true, true, "RFC1918"},
		{"172.16.9.9", true, true, "RFC1918"},
		{"169.254.169.254", true, true, "cloud metadata link-local"},
		{"169.254.169.254", false, true, "metadata link-local always denied"},
		{"::1", true, true, "ipv6 loopback"},
		{"fe80::1", true, true, "ipv6 link-local"},
		{"fc00::1", true, true, "ipv6 ULA (private)"},
		{"::ffff:127.0.0.1", true, true, "ipv4-mapped loopback bypass closed"},
		{"::ffff:a9fe:a9fe", true, true, "ipv4-mapped metadata bypass closed"},
		{"0.0.0.0", true, true, "unspecified"},
		{"224.0.0.1", true, true, "multicast"},
		{"203.0.113.7", true, true, "operator deny CIDR"},
		{"203.0.113.7", false, true, "operator deny CIDR regardless of denyPrivate"},
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		got, why := ipDenied(ip, extra, c.denyPrivate)
		if got != c.want {
			t.Errorf("ipDenied(%s, denyPrivate=%v) = %v (%s), want %v [%s]", c.ip, c.denyPrivate, got, why, c.want, c.note)
		}
	}
	if denied, _ := ipDenied(nil, nil, true); !denied {
		t.Error("nil IP must be denied")
	}
}

func TestLiteralIP(t *testing.T) {
	// Valid literals parse; obfuscated/decimal/hex forms do NOT (→ treated as
	// names, which then fail the allowlist — the obfuscated-IP bypass is closed).
	if literalIP("127.0.0.1") == nil {
		t.Error("dotted IPv4 should parse")
	}
	if literalIP("[::1]") == nil {
		t.Error("bracketed IPv6 should parse")
	}
	for _, weird := range []string{"2130706433", "0x7f000001", "0177.0.0.1", "127.1", "not.an.ip"} {
		if literalIP(weird) != nil {
			t.Errorf("literalIP(%q) should be nil (not a canonical IP literal)", weird)
		}
	}
}
