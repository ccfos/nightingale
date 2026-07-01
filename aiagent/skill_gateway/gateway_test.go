package skillgateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
)

func TestNormalizePath(t *testing.T) {
	cases := map[string]string{
		"/alert-rules":             "/alert-rules",
		"alert-rules":              "/alert-rules",
		"/alert-rules?bgid=1":      "/alert-rules",
		"/api/n9e/alert-rules":     "/alert-rules",
		"/api/n9e/dashboards#frag": "/dashboards",
		"/a/../../etc/passwd":      "/etc/passwd", // path.Clean collapses .. (still blacklist-checked)
		"/targets/":                "/targets",
	}
	for in, want := range cases {
		if got := normalizePath(in); got != want {
			t.Errorf("normalizePath(%q)=%q, want %q", in, got, want)
		}
	}
}

// testGateway builds a Gateway without Start() (which needs a DB) so we can
// exercise the gate + proxy against a mock upstream.
func testGateway(baseURL string, denyExtra []string) *Gateway {
	return &Gateway{
		execID:      "t",
		skill:       "s",
		user:        &models.User{Username: "root"},
		limiter:     newTokenBucket(1000, 1000),
		baseURL:     strings.TrimRight(baseURL, "/"),
		tokenHeader: defaultTokenHeader,
		token:       "tok-123",
		denyPaths:   mergeDenyPaths(denyExtra),
		client:      &http.Client{Timeout: 5 * time.Second},
	}
}

func TestCheckAllowed(t *testing.T) {
	g := testGateway("http://x", []string{"/secret-cfg"})

	if err := g.checkAllowed("GET", "/alert-rules"); err != nil {
		t.Errorf("GET /alert-rules should pass: %v", err)
	}
	// Teams (user-groups) must NOT be caught by the /user/ or /users deny entries.
	if err := g.checkAllowed("GET", "/user-groups"); err != nil {
		t.Errorf("GET /user-groups should pass (not a deny prefix): %v", err)
	}
	for _, p := range []string{
		"/datasources", "/datasource/1", "/notify-channels", "/users", "/user/9", "/sso/config",
		// secret-bearing reads under /api/n9e that are NOT under the /notify-channel
		// or /datasource prefixes — each leaks credentials if reachable:
		"/notify-config",         // SMTP host/user/PASS, IM tokens
		"/config",                // config-center KV (incl. smtp)
		"/user-variable-configs", // plaintext user variables
	} {
		if err := g.checkAllowed("GET", p); err == nil {
			t.Errorf("GET %s must be blacklisted", p)
		}
	}
	if err := g.checkAllowed("GET", "/secret-cfg/x"); err == nil {
		t.Error("operator deny prefix /secret-cfg should block")
	}
	// Percent-encoding must be rejected: "/%64atasources" decodes to "/datasources"
	// upstream and would otherwise dodge the "/datasource" deny prefix.
	for _, p := range []string{"/%64atasources", "/datasource%2f1", "/alert-rules%00"} {
		if err := g.checkAllowed("GET", p); err == nil {
			t.Errorf("percent-encoded path %q must be rejected", p)
		}
	}
	for _, m := range []string{"POST", "PUT", "DELETE", "PATCH"} {
		if err := g.checkAllowed(m, "/alert-rules"); err == nil {
			t.Errorf("%s must be denied (read-only gateway)", m)
		}
	}
}

// TestSafeMethodLabel pins the metric-cardinality guard: known verbs (and the
// rate-limited "-" marker) pass through, any skill-supplied junk collapses to
// "other" so a script can't mint unbounded label values via req.Method.
func TestSafeMethodLabel(t *testing.T) {
	for _, m := range []string{"GET", "POST", "PUT", "DELETE", "PATCH", "-"} {
		if got := safeMethodLabel(m); got != m {
			t.Errorf("safeMethodLabel(%q)=%q, want passthrough", m, got)
		}
	}
	for _, m := range []string{"ZZZRANDOM", "", "GET ", "get", strings.Repeat("X", 4096)} {
		if got := safeMethodLabel(m); got != "other" {
			t.Errorf("safeMethodLabel(%q)=%q, want other", m, got)
		}
	}
}

func TestProxyRoundTrip(t *testing.T) {
	var gotPath, gotToken, gotQuery string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get(defaultTokenHeader)
		gotQuery = r.URL.RawQuery
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"dat":[{"id":1}],"err":""}`))
	}))
	defer upstream.Close()

	g := testGateway(upstream.URL, nil)
	resp := g.handleRequest([]byte(`{"method":"GET","path":"/alert-rules","query":{"bgid":"2"}}`))

	if !resp.OK || resp.Status != 200 {
		t.Fatalf("round-trip failed: %+v", resp)
	}
	if gotPath != "/api/n9e/alert-rules" {
		t.Errorf("upstream path = %q, want /api/n9e/alert-rules", gotPath)
	}
	if gotToken != "tok-123" {
		t.Errorf("upstream token header = %q, want tok-123", gotToken)
	}
	if gotQuery != "bgid=2" {
		t.Errorf("upstream query = %q, want bgid=2", gotQuery)
	}
	if _, ok := resp.Data.(map[string]any); !ok {
		t.Errorf("data should be parsed JSON object, got %T", resp.Data)
	}
}

func TestProxyBlacklistedNotForwarded(t *testing.T) {
	called := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer upstream.Close()

	g := testGateway(upstream.URL, nil)
	resp := g.handleRequest([]byte(`{"method":"GET","path":"/datasources"}`))
	if resp.OK || resp.Error == "" {
		t.Fatalf("blacklisted path should be denied, got %+v", resp)
	}
	if called {
		t.Error("blacklisted request must NOT reach upstream")
	}
}

func TestProxyWriteDenied(t *testing.T) {
	g := testGateway("http://unused", nil)
	resp := g.handleRequest([]byte(`{"method":"DELETE","path":"/alert-rule/1"}`))
	if resp.OK || !strings.Contains(resp.Error, "read-only") {
		t.Fatalf("DELETE should be denied as read-only, got %+v", resp)
	}
}

func TestUpstreamErrorStatus(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"err":"forbidden"}`))
	}))
	defer upstream.Close()
	g := testGateway(upstream.URL, nil)
	resp := g.handleRequest([]byte(`{"method":"GET","path":"/alert-rules"}`))
	if resp.OK || resp.Status != 403 {
		t.Fatalf("403 upstream should surface as not-ok status 403, got %+v", resp)
	}
}

func TestRateLimit(t *testing.T) {
	g := testGateway("http://x", nil)
	g.limiter = newTokenBucket(0, 1) // 1 token, no refill
	_ = g.handleRequest([]byte(`{"method":"GET","path":"/datasources"}`))
	resp := g.handleRequest([]byte(`{"method":"GET","path":"/datasources"}`))
	if !strings.Contains(resp.Error, "rate limit") {
		t.Fatalf("second call should be rate-limited, got %+v", resp)
	}
}

func TestResponseJSON(t *testing.T) {
	b, _ := json.Marshal(response{OK: true, Status: 200, Data: map[string]any{"x": 1}})
	if !strings.Contains(string(b), `"ok":true`) {
		t.Fatalf("bad response json: %s", b)
	}
}
