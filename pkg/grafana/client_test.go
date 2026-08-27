package grafana

import (
	"errors"
	"io"
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

	const listPayload = `[
		{"id":1,"uid":"a","name":"prom","type":"prometheus","url":"http://prom:9090","basicAuth":false},
		{"id":2,"uid":"b","name":"mydb","type":"mysql","url":"10.0.0.1:3306","user":"root","database":"app","basicAuth":true,"basicAuthUser":"admin"}
	]`
	// 详情接口补齐列表里省略的 basicAuthUser / secureJsonFields。
	const promDetail = `{"id":1,"uid":"a","name":"prom","type":"prometheus","url":"http://prom:9090","basicAuth":true,"basicAuthUser":"prom-reader","secureJsonFields":{"basicAuthPassword":true}}`
	const mysqlDetail = `{"id":2,"uid":"b","name":"mydb","type":"mysql","url":"10.0.0.1:3306","user":"root","database":"app","basicAuth":true,"basicAuthUser":"admin"}`

	var listAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/datasources":
			listAuth = r.Header.Get("Authorization") // 列表请求先于并发详情，单线程写入无竞态
			w.Write([]byte(listPayload))
		case "/api/datasources/uid/a":
			w.Write([]byte(promDetail))
		case "/api/datasources/uid/b":
			w.Write([]byte(mysqlDetail))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	lst, err := FetchDatasources(Conn{URL: srv.URL + "/", AuthType: AuthTypeToken, Token: "sa-token"})
	if err != nil {
		t.Fatalf("FetchDatasources: %v", err)
	}
	if listAuth != "Bearer sa-token" {
		t.Errorf("list auth header = %q, want Bearer sa-token", listAuth)
	}
	if len(lst) != 2 {
		t.Fatalf("got %d datasources, want 2", len(lst))
	}
	// 详情补齐：prom 列表里 basicAuth=false、无用户名，详情回填后应带上用户名与 secureJsonFields。
	if !lst[0].BasicAuth || lst[0].BasicAuthUser != "prom-reader" {
		t.Errorf("prom not enriched from detail: basicAuth=%v user=%q", lst[0].BasicAuth, lst[0].BasicAuthUser)
	}
	if !lst[0].SecureJSONFields["basicAuthPassword"] {
		t.Errorf("prom secureJsonFields not enriched: %+v", lst[0].SecureJSONFields)
	}
	if lst[1].Type != "mysql" || lst[1].User != "root" || lst[1].Database != "app" || lst[1].BasicAuthUser != "admin" {
		t.Errorf("mysql datasource wrong: %+v", lst[1])
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

func TestFetchDatasources_MetadataBlocked(t *testing.T) {
	// 云元数据 / link-local 默认拦截(无需服务器，dialControl 在拨号前就拒绝)。
	_, err := FetchDatasources(Conn{URL: "http://169.254.169.254", AuthType: AuthTypeToken, Token: "x"})
	if err == nil {
		t.Fatal("expected cloud metadata address to be blocked")
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

func TestDescribeStatusError(t *testing.T) {
	mk := func(code int) *http.Response {
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader("raw body"))}
	}
	for code, want := range map[int]string{401: "401", 403: "403", 404: "404"} {
		if err := describeStatusError(mk(code)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("status %d -> %v, want friendly msg containing %q", code, err, want)
		}
	}
	// 其它状态保留截断正文，但仍是干净字符串（非结构体 JSON）。
	if err := describeStatusError(mk(500)); err == nil || !strings.Contains(err.Error(), "raw body") {
		t.Errorf("500 -> %v", err)
	}
}

func TestDescribeConnError(t *testing.T) {
	cases := map[string]string{
		"dial tcp 1.2.3.4:3000: connect: connection refused": "连接被拒绝",
		"dial tcp: lookup bad.host: no such host":            "无法解析",
		"x509: certificate signed by unknown authority":      "证书",
		"blocked internal address 127.0.0.1":                 "白名单",
	}
	for in, want := range cases {
		if err := describeConnError(errors.New(in)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("describeConnError(%q) = %v, want contains %q", in, err, want)
		}
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
		// 仍拦截：link-local(含云元数据)、IPv6 元数据、unspecified、组播。
		{mk("169.254.169.254"), none, true}, // cloud metadata (link-local)
		{mk("fd00:ec2::254"), none, true},   // ipv6 cloud metadata
		{mk("fe80::1"), none, true},         // ipv6 link-local
		{mk("fe80::1%eth0"), none, true},    // zoned link-local
		{mk("0.0.0.0"), none, true},         // unspecified
		{mk("ff02::1"), none, true},         // multicast
		// 默认放行：loopback / 私网 / 公网(Grafana 常与 n9e 同机或同内网)。
		{mk("127.0.0.1"), none, false},            // loopback
		{mk("::1"), none, false},                  // ipv6 loopback
		{mk("10.0.0.5"), none, false},             // private
		{mk("192.168.1.10"), none, false},         // private
		{mk("100.64.0.1"), none, false},           // CGNAT
		{mk("::ffff:10.0.0.1"), none, false},      // v4-mapped private
		{mk("8.8.8.8"), none, false},              // public v4
		{mk("2606:4700:4700::1111"), none, false}, // public v6
		// 白名单强制放行(可覆盖默认被拦的地址)。
		{mk("169.254.169.254"), parseAllowlist("169.254.169.254/32"), false}, // allowlist overrides metadata block
		{mk("fe80::1%eth0"), parseAllowlist("fe80::1/128"), false},           // zoned addr hits allowlist
		{mk("169.254.169.254"), parseAllowlist("8.8.8.8/32"), true},          // allowlist miss still blocked
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
