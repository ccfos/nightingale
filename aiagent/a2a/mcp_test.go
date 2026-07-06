package a2a

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	mcpclient "github.com/n9e/n9e-mcp-server/pkg/client"
	mcptoolset "github.com/n9e/n9e-mcp-server/pkg/toolset"
)

// TestInProcRoundTripper verifies the transport hands the outbound request to
// the dispatcher with path/method/query/token intact and returns its response.
func TestInProcRoundTripper(t *testing.T) {
	var gotPath, gotMethod, gotToken, gotQuery, gotRemote string
	dispatcher := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotToken = r.Header.Get("X-User-Token")
		gotQuery, gotRemote = r.URL.RawQuery, r.RemoteAddr
		w.Header().Set("X-Request-Id", "req-1")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"dat":"ok"}`))
	})

	rt := &inProcRoundTripper{dispatcher: dispatcher}
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1/api/n9e/alert-mutes?bgid=9", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-User-Token", "tok-abc")

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if gotPath != "/api/n9e/alert-mutes" || gotMethod != http.MethodPost || gotToken != "tok-abc" || gotQuery != "bgid=9" {
		t.Fatalf("dispatcher saw path=%q method=%q token=%q query=%q", gotPath, gotMethod, gotToken, gotQuery)
	}
	if gotRemote == "" {
		t.Fatal("RemoteAddr not defaulted for downstream middleware")
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	if resp.Header.Get("X-Request-Id") != "req-1" {
		t.Fatalf("response header not propagated: %q", resp.Header.Get("X-Request-Id"))
	}
	if body, _ := io.ReadAll(resp.Body); string(body) != `{"dat":"ok"}` {
		t.Fatalf("body = %q", body)
	}
}

// TestInProcRoundTripperCapsResponseSize proves an oversized internal response
// is bounded: the transport stops buffering at the cap and returns a small,
// non-retryable 413 instead of ballooning the heap with the whole payload.
func TestInProcRoundTripperCapsResponseSize(t *testing.T) {
	chunk := bytes.Repeat([]byte("x"), 1<<20) // 1 MiB, reused across writes
	dispatcher := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		for written := 0; written <= mcpMaxInternalResponseBytes; written += len(chunk) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	})
	rt := &inProcRoundTripper{dispatcher: dispatcher}

	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/api/n9e/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized response: status = %d, want 413", resp.StatusCode)
	}
	// The handed-back body must be the tiny error, not the capped 64 MiB buffer.
	if resp.ContentLength >= 4096 {
		t.Fatalf("overflow body should be small, got %d bytes", resp.ContentLength)
	}
	if body, _ := io.ReadAll(resp.Body); !strings.Contains(string(body), "exceeded") {
		t.Fatalf("overflow body should explain the cap: %s", body)
	}
}

// TestClientForwardsTokenThroughTransport exercises the full X-User-Token path:
// a client built with the in-process transport must reach the dispatcher with
// the caller's token, parse the n9e envelope on success, and surface errors.
func TestClientForwardsTokenThroughTransport(t *testing.T) {
	dispatcher := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-User-Token") != "tok-xyz" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"err":"forbidden"}`))
			return
		}
		if r.URL.Path != "/api/n9e/alert-cur-events/list" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"err":"not found"}`))
			return
		}
		_, _ = w.Write([]byte(`{"dat":{"total":2},"err":""}`))
	})
	transport := &inProcRoundTripper{dispatcher: dispatcher}

	c, err := mcpclient.NewClientWithHTTPClient("tok-xyz", mcpInternalBaseURL, mcpUserAgent, &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	got, err := mcpclient.DoGet[map[string]any](c, context.Background(), "/api/n9e/alert-cur-events/list", nil)
	if err != nil {
		t.Fatalf("DoGet: %v", err)
	}
	if got["total"] != float64(2) {
		t.Fatalf("total = %v, want 2", got["total"])
	}

	// A caller whose token the internal route rejects must get an error, not a
	// silent empty result.
	badClient, err := mcpclient.NewClientWithHTTPClient("wrong", mcpInternalBaseURL, mcpUserAgent, &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mcpclient.DoGet[map[string]any](badClient, context.Background(), "/api/n9e/alert-cur-events/list", nil); err == nil {
		t.Fatal("expected error for rejected token, got nil")
	}
}

// TestBuildMCPServerToolsListing connects an in-memory MCP client to the built
// server and asserts the fine-grained toolset is what /mcp exposes now: the old
// single n9e_assistant tool is gone, the whitelist is honored, and read-only
// mode drops write tools.
func TestBuildMCPServerToolsListing(t *testing.T) {
	httpClient := &http.Client{Transport: &inProcRoundTripper{dispatcher: http.NotFoundHandler()}}

	full := listToolNames(t, buildMCPServer(httpClient, []string{"mutes"}, false))
	if contains(full, "n9e_assistant") {
		t.Fatal("legacy n9e_assistant tool should be removed")
	}
	if !contains(full, "list_mutes") || !contains(full, "create_mute") {
		t.Fatalf("mutes toolset not fully registered: %v", full)
	}
	if contains(full, "list_dashboards") {
		t.Fatalf("non-whitelisted toolset leaked: %v", full)
	}

	readOnly := listToolNames(t, buildMCPServer(httpClient, []string{"mutes"}, true))
	if contains(readOnly, "create_mute") || contains(readOnly, "update_mute") {
		t.Fatalf("read-only server should drop write tools: %v", readOnly)
	}
	if !contains(readOnly, "list_mutes") {
		t.Fatalf("read-only server should keep read tools: %v", readOnly)
	}
}

// TestResolveToolsets pins the whitelist semantics: empty means the center
// default (all except the by-default-excluded metrics); the explicit "all"
// keyword opts into everything; a valid list passes through; and an unknown
// name is dropped — a typo must shrink the exposed set, never widen a
// restricted whitelist to all toolsets (which would also re-expose write tools).
func TestResolveToolsets(t *testing.T) {
	all := len(mcptoolset.DefaultToolsets)

	// Empty → every default toolset, metrics included (its tools decode the
	// native Prometheus envelope since n9e-mcp-server's doPromGet).
	if got := resolveToolsets(MCPConfig{}); len(got) != all || !contains(got, "metrics") {
		t.Fatalf("empty whitelist should be all default toolsets incl. metrics: %v", got)
	}
	// "all" is kept as a config-back-compat synonym for the default.
	if got := resolveToolsets(MCPConfig{Toolsets: []string{"all"}}); len(got) != all || !contains(got, "metrics") {
		t.Fatalf("explicit all should include metrics: %v", got)
	}
	if got := resolveToolsets(MCPConfig{Toolsets: []string{"metrics"}}); len(got) != 1 || got[0] != "metrics" {
		t.Fatalf("metrics-only whitelist should be honored: %v", got)
	}
	if got := resolveToolsets(MCPConfig{Toolsets: []string{"mutes"}}); len(got) != 1 || got[0] != "mutes" {
		t.Fatalf("valid whitelist should pass through: %v", got)
	}
	// The core fail-closed case: a typo drops that name, keeping the rest.
	if got := resolveToolsets(MCPConfig{Toolsets: []string{"mutes", "dashbaords"}}); len(got) != 1 || got[0] != "mutes" {
		t.Fatalf("typo should be dropped, not widen to all; got %v", got)
	}
	if got := resolveToolsets(MCPConfig{Toolsets: []string{"nope"}}); len(got) != 0 {
		t.Fatalf("all-unknown whitelist should enable nothing; got %v", got)
	}

	// End-to-end: a typo-only whitelist must expose zero tools, never fall open
	// to the full (write-capable) set.
	httpClient := &http.Client{Transport: &inProcRoundTripper{dispatcher: http.NotFoundHandler()}}
	enabled := resolveToolsets(MCPConfig{Toolsets: []string{"dashbaords"}})
	if names := listToolNames(t, buildMCPServer(httpClient, enabled, false)); len(names) != 0 {
		t.Fatalf("typo-only whitelist should expose no tools; got %v", names)
	}

	// Sanity: NewMCPHandler tolerates a bad whitelist without panicking.
	if h := NewMCPHandler(http.NotFoundHandler(), MCPConfig{Toolsets: []string{"nope"}}); h == nil {
		t.Fatal("handler is nil for invalid toolset config")
	}
}

// TestMCPHandlerRejectsMissingToken proves a caller without the TokenAuth header
// (e.g. a Bearer/OAuth caller that passed the edge auth) is rejected up front
// with a clear 401, instead of getting an MCP session whose every tool call
// would fail the internal hop.
func TestMCPHandlerRejectsMissingToken(t *testing.T) {
	h := NewMCPHandler(http.NotFoundHandler(), MCPConfig{})

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token: status = %d, want 401; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "X-User-Token") {
		t.Fatalf("401 body should name the required header: %s", rec.Body.String())
	}
}

// TestInProcRoundTripperBearerCredential covers the OAuth path: when the
// request context carries a bearer credential, the transport must move the
// token from the client's hardcoded X-User-Token stamp to Authorization:
// Bearer (and not replay it under the configured TokenAuth header), so the
// internal tokenAuth verifies it as an OAuth access token.
func TestInProcRoundTripperBearerCredential(t *testing.T) {
	var gotAuthz, gotUserToken, gotConfigured string
	var gotMarker bool
	dispatcher := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthz = r.Header.Get("Authorization")
		gotUserToken = r.Header.Get("X-User-Token")
		gotConfigured = r.Header.Get("X-Auth-Token")
		gotMarker = IsMCPInProcDispatch(r.Context())
		_, _ = w.Write([]byte(`{"dat":"ok"}`))
	})
	rt := &inProcRoundTripper{dispatcher: dispatcher, tokenHeader: "X-Auth-Token"}

	ctx := context.WithValue(context.Background(), mcpTokenCtxKey{}, mcpCredential{token: "acc-tok", bearer: true})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1/api/n9e/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-User-Token", "acc-tok") // what n9e-mcp-server's client sets
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if gotAuthz != "Bearer acc-tok" {
		t.Fatalf("Authorization = %q, want \"Bearer acc-tok\"", gotAuthz)
	}
	if gotUserToken != "" || gotConfigured != "" {
		t.Fatalf("bearer credential must not ride token headers: X-User-Token=%q X-Auth-Token=%q", gotUserToken, gotConfigured)
	}
	if !gotMarker {
		t.Fatal("internal hop lost the in-process dispatch marker")
	}
}

// TestIsMCPInProcDispatch pins the marker contract tokenAuth relies on: true
// exactly when the context went through NewMCPHandler's credential stash.
func TestIsMCPInProcDispatch(t *testing.T) {
	if IsMCPInProcDispatch(context.Background()) {
		t.Fatal("bare context must not read as in-process dispatch")
	}
	ctx := context.WithValue(context.Background(), mcpTokenCtxKey{}, mcpCredential{token: "x"})
	if !IsMCPInProcDispatch(ctx) {
		t.Fatal("stashed credential must mark the context as in-process dispatch")
	}
}

// TestMCPHandlerAcceptsBearerAtEntry ensures an Authorization-only caller gets
// past the credential-presence backstop (the MCP protocol layer answers, not
// the 401 gate).
func TestMCPHandlerAcceptsBearerAtEntry(t *testing.T) {
	h := NewMCPHandler(http.NotFoundHandler(), MCPConfig{})

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer acc-tok")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("bearer caller rejected at entry: %s", rec.Body.String())
	}
}

// TestInProcRoundTripperReplaysConfiguredHeader covers a non-default TokenAuth
// header: the client always sets X-User-Token, so the transport must also place
// the token under the configured header for the internal tokenAuth to see it.
func TestInProcRoundTripperReplaysConfiguredHeader(t *testing.T) {
	var gotConfigured string
	dispatcher := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotConfigured = r.Header.Get("X-Auth-Token")
		_, _ = w.Write([]byte(`{"dat":"ok"}`))
	})
	rt := &inProcRoundTripper{dispatcher: dispatcher, tokenHeader: "X-Auth-Token"}

	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/api/n9e/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-User-Token", "tok-1") // what n9e-mcp-server's client sets
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if gotConfigured != "tok-1" {
		t.Fatalf("token not replayed under configured header: X-Auth-Token=%q", gotConfigured)
	}
}

func listToolNames(t *testing.T, server *mcp.Server) []string {
	t.Helper()
	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	ss, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer ss.Close()

	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer cs.Close()

	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func contains(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}
