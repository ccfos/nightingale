package a2a

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// e2eToken is the credential the fake internal tokenAuth accepts. A tool call
// only reaches the routes if the in-process hop replays this same token.
const e2eToken = "tok-e2e"

// e2eBearer is the OAuth access token the fake tokenAuth accepts — but, like
// the real one, only on requests carrying the in-process dispatch marker.
const e2eBearer = "oauth-acc-e2e"

// e2eState records what the internal /api/n9e routes observed, so the test can
// prove a tool call actually dispatched in-process with the caller's identity.
type e2eState struct {
	sawToken      string
	sawAuthz      string
	sawMarker     bool
	lastListGroup string
	createdNote   string
}

// newMCPTestServer stands up a gin engine that mirrors the center mount: real
// /api/n9e routes behind a token gate, and /mcp serving the fine-grained
// toolset that dispatches back into this same engine. It returns a live HTTP
// server so a real MCP streamable client can drive the full protocol.
func newMCPTestServer(t *testing.T, cfg MCPConfig) (*httptest.Server, *e2eState) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	st := &e2eState{}
	r := gin.New()

	// Internal API gate mirroring the real tokenAuth: a fixed X-User-Token is
	// always accepted; an OAuth Bearer token is accepted ONLY on requests
	// carrying the in-process dispatch marker (agent-plane replay) — a Bearer
	// caller hitting the API directly from outside stays rejected.
	api := r.Group("/api/n9e")
	api.Use(func(c *gin.Context) {
		st.sawToken = c.GetHeader("X-User-Token")
		st.sawAuthz = c.GetHeader("Authorization")
		st.sawMarker = IsMCPInProcDispatch(c.Request.Context())
		switch {
		case st.sawToken == e2eToken:
		case st.sawAuthz == "Bearer "+e2eBearer && st.sawMarker:
		default:
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"err": "no permission"})
			return
		}
		c.Next()
	})
	api.GET("/busi-group/:id/alert-mutes", func(c *gin.Context) {
		st.lastListGroup = c.Param("id")
		c.JSON(http.StatusOK, gin.H{"dat": []gin.H{{"id": 1}}, "err": ""})
	})
	api.POST("/busi-group/:id/alert-mutes", func(c *gin.Context) {
		var body map[string]any
		_ = c.ShouldBindJSON(&body)
		if n, ok := body["note"].(string); ok {
			st.createdNote = n
		}
		c.JSON(http.StatusOK, gin.H{"dat": 1001, "err": ""})
	})

	// /mcp on the SAME engine r, gated by a credential check like the real
	// chain (which accepts TokenAuth and, via agentOAuthScope, OAuth Bearer).
	mcpHandler := http.StripPrefix("/mcp", NewMCPHandler(r, cfg))
	mcpGroup := r.Group("/mcp")
	mcpGroup.Use(func(c *gin.Context) {
		if c.GetHeader("X-User-Token") == "" && c.GetHeader("Authorization") == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"err": "unauthorized"})
			return
		}
		c.Next()
	})
	mcpGroup.Any("", gin.WrapH(mcpHandler))
	mcpGroup.Any("/*proxyPath", gin.WrapH(mcpHandler))

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, st
}

// connectMCP dials the /mcp endpoint as a real MCP streamable client, sending
// the given X-User-Token on every request.
func connectMCP(t *testing.T, ctx context.Context, endpoint, token string) *mcp.ClientSession {
	t.Helper()
	hc := &http.Client{Transport: &headerRoundTripper{base: http.DefaultTransport, token: token}}
	transport := &mcp.StreamableClientTransport{
		Endpoint:             endpoint,
		HTTPClient:           hc,
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "e2e", Version: "0"}, nil).Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("mcp connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

type headerRoundTripper struct {
	base   http.RoundTripper
	token  string
	bearer bool // send as Authorization: Bearer instead of X-User-Token
}

func (t *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	if t.bearer {
		req.Header.Set("Authorization", "Bearer "+t.token)
	} else {
		req.Header.Set("X-User-Token", t.token)
	}
	return t.base.RoundTrip(req)
}

// connectMCPBearer dials /mcp as an OAuth client: the only credential on the
// wire is an Authorization: Bearer access token.
func connectMCPBearer(t *testing.T, ctx context.Context, endpoint, accessToken string) *mcp.ClientSession {
	t.Helper()
	hc := &http.Client{Transport: &headerRoundTripper{base: http.DefaultTransport, token: accessToken, bearer: true}}
	transport := &mcp.StreamableClientTransport{
		Endpoint:             endpoint,
		HTTPClient:           hc,
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "e2e-oauth", Version: "0"}, nil).Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("mcp connect (bearer): %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// TestMCPEndToEndOverHTTP drives the full MCP protocol against a live /mcp: it
// lists the fine-grained toolset, then calls a read tool and a write tool and
// asserts each dispatched in-process into the real /api/n9e route carrying the
// caller's X-User-Token.
func TestMCPEndToEndOverHTTP(t *testing.T) {
	ctx := context.Background()
	srv, st := newMCPTestServer(t, MCPConfig{Toolsets: []string{"mutes", "dashboards"}})
	cs := connectMCP(t, ctx, srv.URL+"/mcp", e2eToken)

	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	if contains(names, "n9e_assistant") {
		t.Fatalf("legacy n9e_assistant should be gone: %v", names)
	}
	for _, want := range []string{"list_mutes", "create_mute", "list_dashboards"} {
		if !contains(names, want) {
			t.Fatalf("tool %q missing from tools/list: %v", want, names)
		}
	}

	// Read tool → GET /api/n9e/busi-group/7/alert-mutes, in-process.
	readRes, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_mutes",
		Arguments: map[string]any{"group_id": 7},
	})
	if err != nil {
		t.Fatalf("call list_mutes: %v", err)
	}
	if readRes.IsError {
		t.Fatalf("list_mutes returned tool error: %s", toolText(readRes))
	}
	if st.lastListGroup != "7" {
		t.Fatalf("internal route saw group %q, want 7", st.lastListGroup)
	}
	if st.sawToken != e2eToken {
		t.Fatalf("internal route saw token %q, want the caller's %q", st.sawToken, e2eToken)
	}

	// Write tool → POST the note through to the same route.
	writeRes, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_mute",
		Arguments: map[string]any{
			"group_id": 7, "note": "written-by-e2e", "cause": "c", "btime": 1, "etime": 2,
		},
	})
	if err != nil {
		t.Fatalf("call create_mute: %v", err)
	}
	if writeRes.IsError {
		t.Fatalf("create_mute returned tool error: %s", toolText(writeRes))
	}
	if st.createdNote != "written-by-e2e" {
		t.Fatalf("internal route recorded note %q, want written-by-e2e", st.createdNote)
	}
}

// TestMCPEndToEndRBACRejectsForeignToken proves the caller identity is enforced
// on the internal hop: a client whose token the /api/n9e route rejects gets a
// tool error, not another user's data.
func TestMCPEndToEndRBACRejectsForeignToken(t *testing.T) {
	ctx := context.Background()
	srv, _ := newMCPTestServer(t, MCPConfig{Toolsets: []string{"mutes"}})
	cs := connectMCP(t, ctx, srv.URL+"/mcp", "foreign-token")

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_mutes",
		Arguments: map[string]any{"group_id": 7},
	})
	if err != nil {
		t.Fatalf("call list_mutes: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected a tool error for a foreign token, got %+v", res)
	}
}

// TestMCPEndToEndOverHTTPWithBearer drives the full MCP protocol as an OAuth
// caller: the only credential is an Authorization: Bearer access token. The
// internal hop must present exactly that credential (no X-User-Token) plus the
// in-process dispatch marker, and the tool call must succeed.
func TestMCPEndToEndOverHTTPWithBearer(t *testing.T) {
	ctx := context.Background()
	srv, st := newMCPTestServer(t, MCPConfig{Toolsets: []string{"mutes"}})
	cs := connectMCPBearer(t, ctx, srv.URL+"/mcp", e2eBearer)

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_mutes",
		Arguments: map[string]any{"group_id": 42},
	})
	if err != nil {
		t.Fatalf("call list_mutes: %v", err)
	}
	if res.IsError {
		t.Fatalf("list_mutes returned tool error: %s", toolText(res))
	}
	if st.lastListGroup != "42" {
		t.Fatalf("internal route saw group %q, want 42", st.lastListGroup)
	}
	if st.sawAuthz != "Bearer "+e2eBearer {
		t.Fatalf("internal route saw Authorization %q, want the caller's bearer token", st.sawAuthz)
	}
	if st.sawToken != "" {
		t.Fatalf("bearer flow must not fabricate an X-User-Token, saw %q", st.sawToken)
	}
	if !st.sawMarker {
		t.Fatal("internal route did not see the in-process dispatch marker")
	}
}

// TestMCPEndToEndBearerRejectedOutsideDispatch proves the isolation the marker
// buys: the same OAuth token that works through /mcp is rejected when thrown
// directly at the internal API from outside (no in-process dispatch marker).
func TestMCPEndToEndBearerRejectedOutsideDispatch(t *testing.T) {
	srv, st := newMCPTestServer(t, MCPConfig{Toolsets: []string{"mutes"}})

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/n9e/busi-group/7/alert-mutes", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+e2eBearer)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("direct external bearer call: status = %d, want 403", resp.StatusCode)
	}
	if st.sawMarker {
		t.Fatal("external request must not carry the in-process dispatch marker")
	}
}

func toolText(res *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}
