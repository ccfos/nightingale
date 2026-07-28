package grafana

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
)

func TestFetchDatasources(t *testing.T) {
	// httptest 监听 127.0.0.1，默认拒绝内网，需白名单放行 loopback。
	t.Setenv(allowlistEnv, "127.0.0.1/32,::1/128")

	const payload = `[
		{"id":1,"uid":"a","name":"prom","type":"prometheus","url":"http://prom:9090","basicAuth":false},
		{"id":2,"uid":"b","name":"mydb","type":"mysql","url":"10.0.0.1:3306","user":"root","database":"app","basicAuth":true,"basicAuthUser":"admin"}
	]`

	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(payload))
	}))
	defer srv.Close()

	lst, err := FetchDatasources(Conn{URL: srv.URL + "/", AuthType: AuthTypeToken, Token: "sa-token"})
	if err != nil {
		t.Fatalf("FetchDatasources: %v", err)
	}
	if gotPath != "/api/datasources" {
		t.Errorf("path = %q, want /api/datasources", gotPath)
	}
	if gotAuth != "Bearer sa-token" {
		t.Errorf("auth header = %q, want Bearer sa-token", gotAuth)
	}
	if len(lst) != 2 {
		t.Fatalf("got %d datasources, want 2", len(lst))
	}
	if lst[1].Type != "mysql" || lst[1].User != "root" || lst[1].Database != "app" || !lst[1].BasicAuth {
		t.Errorf("second datasource parsed wrong: %+v", lst[1])
	}
}

func TestFetchDatasources_HTTPError(t *testing.T) {
	t.Setenv(allowlistEnv, "127.0.0.1/32,::1/128")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := FetchDatasources(Conn{URL: srv.URL, AuthType: AuthTypeToken, Token: "bad"})
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
}

func TestFetchDatasources_InternalBlocked(t *testing.T) {
	// 不设白名单：连 loopback 的 httptest 也应被拦截（secure-by-default）。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	_, err := FetchDatasources(Conn{URL: srv.URL, AuthType: AuthTypeToken, Token: "x"})
	if err == nil {
		t.Fatal("expected loopback to be blocked without allowlist")
	}
}

func TestFetchDatasources_TooLarge(t *testing.T) {
	t.Setenv(allowlistEnv, "127.0.0.1/32,::1/128")
	orig := maxResponseBytes
	maxResponseBytes = 16
	defer func() { maxResponseBytes = orig }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"name":"aaaaaaaaaaaaaaaaaaaaaaaaaaaa","type":"prometheus"}]`))
	}))
	defer srv.Close()

	_, err := FetchDatasources(Conn{URL: srv.URL, AuthType: AuthTypeToken, Token: "x"})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected 'exceeds' error, got %v", err)
	}
}

func TestRedirectPolicy(t *testing.T) {
	mkReq := func(raw string) *http.Request {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("bad url %q: %v", raw, err)
		}
		return &http.Request{URL: u}
	}
	cases := []struct {
		name    string
		to      string
		from    string // via[0]；空表示首个请求（无 via）
		wantErr bool
	}{
		{"first request", "https://g/api/datasources", "", false},
		{"https->https same host", "https://g/x", "https://g/api/datasources", false},
		{"https->http downgrade blocked (credential leak)", "http://g/x", "https://g/api/datasources", true},
		{"http->https upgrade allowed", "https://g/x", "http://g/api/datasources", false},
		{"cross host blocked", "https://evil/x", "https://g/api/datasources", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var via []*http.Request
			if c.from != "" {
				via = []*http.Request{mkReq(c.from)}
			}
			if err := redirectPolicy(mkReq(c.to), via); (err != nil) != c.wantErr {
				t.Fatalf("redirectPolicy err=%v, wantErr=%v", err, c.wantErr)
			}
		})
	}

	// 超过 5 跳一律拒绝。
	via := make([]*http.Request, 5)
	for i := range via {
		via[i] = mkReq("https://g/api/datasources")
	}
	if err := redirectPolicy(mkReq("https://g/x"), via); err == nil {
		t.Fatal("expected error after too many redirects")
	}
}

func TestBuildTargetURL(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"simple", "http://g:3000", "http://g:3000/api/datasources", false},
		{"trailing slash trimmed", "https://g:3000/", "https://g:3000/api/datasources", false},
		{"subpath preserved", "http://host/grafana", "http://host/grafana/api/datasources", false},
		{"query stripped (no path bypass)", "http://host?redirect=/evil", "http://host/api/datasources", false},
		{"fragment stripped", "http://host/g#frag", "http://host/g/api/datasources", false},
		{"reject file scheme", "file:///etc/passwd", "", true},
		{"reject ftp scheme", "ftp://host/x", "", true},
		{"reject empty", "", "", true},
		{"reject missing host", "http://", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildTargetURL(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %q", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("buildTargetURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestTargetBlocked(t *testing.T) {
	mk := netip.MustParseAddr
	var none []netip.Prefix
	cases := []struct {
		ip      netip.Addr
		allow   []netip.Prefix
		blocked bool
	}{
		{mk("169.254.169.254"), none, true},               // cloud metadata (link-local)
		{mk("fd00:ec2::254"), none, true},                 // ipv6 metadata (ULA => private)
		{mk("fe80::1"), none, true},                       // ipv6 link-local
		{mk("fe80::1%eth0"), none, true},                  // zoned link-local — the reported bypass
		{mk("0.0.0.0"), none, true},                       // unspecified
		{mk("127.0.0.1"), none, true},                     // loopback
		{mk("::1"), none, true},                           // ipv6 loopback
		{mk("10.0.0.5"), none, true},                      // private
		{mk("192.168.1.10"), none, true},                  // private
		{mk("100.64.0.1"), none, true},                    // CGNAT (RFC 6598)
		{mk("240.0.0.1"), none, true},                     // reserved
		{mk("192.0.2.5"), none, true},                     // documentation
		{mk("ff02::1"), none, true},                       // multicast
		{mk("::ffff:10.0.0.1"), none, true},               // v4-mapped private
		{mk("fec0::1"), none, true},                       // ipv6 site-local (deprecated, still internal)
		{mk("64:ff9b:1::1"), none, true},                  // ipv6 NAT64 local
		{mk("0.0.0.1"), none, true},                       // 0.0.0.0/8 "this network"
		{mk("8.8.8.8"), none, false},                      // public v4
		{mk("2606:4700:4700::1111"), none, false},         // public v6
		{mk("10.0.0.5"), parseAllowlist("10.0.0.0/8"), false},      // allowlisted subnet
		{mk("127.0.0.1"), parseAllowlist("127.0.0.1/32"), false},   // allowlisted loopback
		{mk("fe80::1%eth0"), parseAllowlist("fe80::1/128"), false}, // zoned addr hits allowlist
		{mk("169.254.169.254"), parseAllowlist("8.8.8.8/32"), true}, // allowlist miss still blocked
	}
	for _, c := range cases {
		if got := targetBlocked(c.ip, c.allow); got != c.blocked {
			t.Errorf("targetBlocked(%s) = %v, want %v", c.ip, got, c.blocked)
		}
	}
}

func TestParseAllowlist(t *testing.T) {
	// valid: 127.0.0.1/32, 10.0.0.0/8, ::1(->/128), 8.8.8.8(->/32); skip empty & bad/x
	nets := parseAllowlist("127.0.0.1/32, 10.0.0.0/8 ,::1, ,bad/x, 8.8.8.8")
	if len(nets) != 4 {
		t.Fatalf("parsed %d prefixes, want 4", len(nets))
	}
	if !nets[0].Contains(netip.MustParseAddr("127.0.0.1")) {
		t.Errorf("first prefix should contain 127.0.0.1")
	}
}

func TestDecodeDatasources(t *testing.T) {
	t.Run("valid array", func(t *testing.T) {
		lst, err := decodeDatasources(strings.NewReader(`[{"name":"a","type":"prometheus"},{"name":"b","type":"mysql"}]`))
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(lst) != 2 || lst[0].Name != "a" || lst[1].Type != "mysql" {
			t.Fatalf("parsed wrong: %+v", lst)
		}
	})
	t.Run("empty array ok", func(t *testing.T) {
		lst, err := decodeDatasources(strings.NewReader(`[]`))
		if err != nil || len(lst) != 0 {
			t.Fatalf("empty array: lst=%v err=%v", lst, err)
		}
	})
	t.Run("not an array", func(t *testing.T) {
		if _, err := decodeDatasources(strings.NewReader(`{"name":"a"}`)); err == nil {
			t.Fatal("expected error for non-array response")
		}
	})
	t.Run("truncated array (no closing bracket)", func(t *testing.T) {
		if _, err := decodeDatasources(strings.NewReader(`[{"name":"a"},`)); err == nil {
			t.Fatal("expected error for truncated array")
		}
	})
	t.Run("truncated after complete element", func(t *testing.T) {
		if _, err := decodeDatasources(strings.NewReader(`[{"name":"a"}`)); err == nil {
			t.Fatal("expected error when closing bracket missing")
		}
	})
	t.Run("trailing content after array", func(t *testing.T) {
		if _, err := decodeDatasources(strings.NewReader(`[{"name":"a"}]garbage`)); err == nil {
			t.Fatal("expected error for trailing content")
		}
	})
	t.Run("element cap enforced", func(t *testing.T) {
		orig := maxDatasources
		maxDatasources = 2
		defer func() { maxDatasources = orig }()
		if _, err := decodeDatasources(strings.NewReader(`[{},{},{}]`)); err == nil {
			t.Fatal("expected error when exceeding element cap")
		}
	})
}
