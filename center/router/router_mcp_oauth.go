package router

// Built-in OAuth 2.1 Authorization Server (范式 A) for the MCP / A2A endpoints.
//
// This makes n9e its own Authorization Server (co-located with the Resource
// Server implemented in router_rsauth.go), so generic MCP clients can connect to
// /a2a /mcp with zero pre-configuration via RFC 7591 Dynamic Client
// Registration — the "no external IdP" counterpart to RSAuth. It is independent
// from RSAuth: both may be enabled and are both advertised in the RFC 9728
// protected-resource metadata.
//
// Design — stateless-first (see doc/api/mcp-oauth-as.md):
//   - client_id, authorization-request ticket, authorization code, access and
//     refresh tokens are ALL HS256 JWTs distinguished by a `token_use` claim and
//     signed with a key derived from JWTAuth.SigningKey (HKDF), cryptographically
//     separate from the session-JWT key — so an MCP token can never be replayed
//     as a session token and vice-versa.
//   - The ONLY shared state is a one-time-use guard for authorization codes in
//     the shared Redis (SetNX at token exchange), making the flow correct across
//     all center instances behind a load balancer.
//   - Refresh tokens are stateless (no rotation); revocation relies on short TTLs
//     or rotating the signing key.
//
// Specs: OAuth 2.1, RFC 8414 (AS metadata), RFC 9728 (protected resource), RFC
// 7591 (DCR), RFC 7636 (PKCE), RFC 8707 (resource indicators), RFC 7009 (revoke).

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"golang.org/x/crypto/hkdf"
)

const (
	// token_use claim values — the discriminator that keeps the five JWT kinds
	// (and the session JWT, which carries none of these) from being confused.
	mcpUseClient   = "mcp_client"
	mcpUseAuthzReq = "mcp_authz_req"
	mcpUseCode     = "mcp_code"
	mcpUseAccess   = "mcp_access"
	mcpUseRefresh  = "mcp_refresh"

	mcpAuthorizePath = "/oauth/authorize"
	mcpTokenPath     = "/oauth/token"
	mcpRegisterPath  = "/oauth/register"
	mcpRevokePath    = "/oauth/revoke"
	mcpASMetaPath    = "/.well-known/oauth-authorization-server"
	// mcpConsentPath is a frontend (SPA) route, not a backend handler — the
	// authorize endpoint 302-redirects the browser here; the SPA (which holds the
	// session token and handles login incl. SSO) renders consent and calls the
	// protected decision API.
	mcpConsentPath = "/oauth-consent"

	mcpScope            = "mcp"
	mcpMaxRedirectURIs  = 5
	mcpMaxClientNameLen = 200
	mcpAuthzReqTTL      = 5 * time.Minute

	mcpCodeRedisPrefix = "/mcp-oauth/code/"
	mcpKeyDeriveInfo   = "n9e-mcp-oauth"

	// mcpMountKey is the gin-context key holding the API mount prefix the request
	// arrived on ("" for the root mount, or a2aMountPrefix for the /api/n9e copy).
	// It lets the AS advertise issuer/endpoint URLs that match the surface the
	// client actually used. Set by withMount.
	mcpMountKey = "a2a_mount"
)

// mcpKeyCache memoizes the HKDF-derived signing key per source key so the hot
// path (access-token verification on every /mcp call) avoids re-deriving.
var mcpKeyCache sync.Map // map[string][]byte

// mcpAuthEnabled reports whether the built-in Authorization Server is usable:
// the switch is on and an effective signing key exists (explicit or derivable
// from the session signing key).
func (rt *Router) mcpAuthEnabled() bool {
	if !rt.HTTP.MCPAuth.Enable {
		return false
	}
	return rt.HTTP.MCPAuth.SigningKey != "" || rt.HTTP.JWTAuth.SigningKey != ""
}

// mcpSigningKey returns the HS256 key for all MCP OAuth JWTs. An explicit
// MCPAuth.SigningKey is used verbatim; otherwise a 32-byte key is derived from
// JWTAuth.SigningKey via HKDF-SHA256 — deterministic across instances and
// cryptographically independent from the session key.
func (rt *Router) mcpSigningKey() []byte {
	if k := strings.TrimSpace(rt.HTTP.MCPAuth.SigningKey); k != "" {
		return []byte(k)
	}
	src := rt.HTTP.JWTAuth.SigningKey
	if v, ok := mcpKeyCache.Load(src); ok {
		return v.([]byte)
	}
	out := make([]byte, 32)
	_, _ = io.ReadFull(hkdf.New(sha256.New, []byte(src), nil, []byte(mcpKeyDeriveInfo)), out)
	mcpKeyCache.Store(src, out)
	return out
}

func (rt *Router) mcpSign(claims jwt.MapClaims) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(rt.mcpSigningKey())
}

// mcpParse verifies a token's signature (with the MCP key, rejecting anything
// not minted by us — incl. the session JWT and external-IdP tokens), validates
// exp via the JWT library, and asserts the token_use claim.
func (rt *Router) mcpParse(raw, wantUse string) (jwt.MapClaims, error) {
	claims := jwt.MapClaims{}
	tok, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return rt.mcpSigningKey(), nil
	})
	if err != nil {
		return nil, err
	}
	if !tok.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	if tu, _ := claims["token_use"].(string); tu != wantUse {
		return nil, fmt.Errorf("token_use mismatch: want %s", wantUse)
	}
	return claims, nil
}

// mcpVerifyAccessToken validates a builtin-AS access token and resolves it to a
// local user. Called from tokenAuth before the external-IdP RS branch; returns
// ok=false (so the request falls through) for anything not our access token.
func (rt *Router) mcpVerifyAccessToken(raw string) (userID int64, username string, ok bool) {
	claims, err := rt.mcpParse(raw, mcpUseAccess)
	if err != nil {
		return 0, "", false
	}
	// Audience binding (RFC 8707) — enforce only when a stable resource id is
	// configured; a request-derived resource can vary by host so the signature
	// (already proven ours) is the binding in that case.
	if want := rt.mcpConfiguredResource(); want != "" {
		if aud, _ := claims["aud"].(string); aud != want {
			return 0, "", false
		}
	}
	uid, err := strconv.ParseInt(mcpClaimString(claims, "sub"), 10, 64)
	if err != nil {
		return 0, "", false
	}
	usr := mcpClaimString(claims, "usr")
	if usr == "" {
		return 0, "", false
	}
	return uid, usr, true
}

// mcpBaseURL is the canonical scheme://host of this service: explicit
// A2A.BaseURL, else derived from the request (honouring X-Forwarded-Proto / TLS).
func (rt *Router) mcpBaseURL(c *gin.Context) string {
	if base := strings.TrimSuffix(rt.HTTP.A2A.BaseURL, "/"); base != "" {
		return base
	}
	scheme := "http"
	if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host
}

// withMount records the API mount prefix the AS endpoint is served under, so the
// metadata/endpoint URLs derived downstream (mcpAPIBaseURL) match the surface the
// request arrived on. Root handlers go unwrapped (mcpMountKey unset → "").
func (rt *Router) withMount(mount string, h gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(mcpMountKey, mount)
		h(c)
	}
}

// mcpAPIBaseURL is mcpBaseURL plus the active API mount prefix, so the built-in
// AS advertises endpoints under whichever surface (root or /api/n9e) the client
// reached it on. NOTE: the interactive consent redirect deliberately uses
// mcpBaseURL (no mount) because /oauth-consent is a root frontend SPA route.
func (rt *Router) mcpAPIBaseURL(c *gin.Context) string {
	return rt.mcpBaseURL(c) + c.GetString(mcpMountKey)
}

// mcpIssuer is this AS's issuer (explicit MCPAuth.Issuer, else the mount-aware
// base URL). Multi-instance deployments should set it explicitly so every
// instance agrees — but doing so pins a single surface, so leave it empty when
// you want both the root and /api/n9e copies to self-advertise.
func (rt *Router) mcpIssuer(c *gin.Context) string {
	if iss := strings.TrimSpace(rt.HTTP.MCPAuth.Issuer); iss != "" {
		return strings.TrimSuffix(iss, "/")
	}
	return rt.mcpAPIBaseURL(c)
}

// mcpResource is the resource id bound into the access token aud — explicit
// MCPAuth.Resource, else RSAuth.Audience, else "<base>/mcp".
func (rt *Router) mcpResource(c *gin.Context) string {
	if r := rt.mcpConfiguredResource(); r != "" {
		return r
	}
	return rt.mcpAPIBaseURL(c) + "/mcp"
}

// mcpConfiguredResource is the statically-configured resource id (no request
// derivation), used where aud must be compared deterministically.
func (rt *Router) mcpConfiguredResource() string {
	if r := strings.TrimSpace(rt.HTTP.MCPAuth.Resource); r != "" {
		return r
	}
	return strings.TrimSpace(rt.HTTP.RSAuth.Audience)
}

func (rt *Router) mcpAccessTTL() time.Duration {
	if rt.HTTP.MCPAuth.AccessTTL > 0 {
		return time.Duration(rt.HTTP.MCPAuth.AccessTTL) * time.Second
	}
	return time.Hour
}

func (rt *Router) mcpRefreshTTL() time.Duration {
	if rt.HTTP.MCPAuth.RefreshTTL > 0 {
		return time.Duration(rt.HTTP.MCPAuth.RefreshTTL) * time.Second
	}
	return 7 * 24 * time.Hour
}

func (rt *Router) mcpCodeTTL() time.Duration {
	if rt.HTTP.MCPAuth.CodeTTL > 0 {
		return time.Duration(rt.HTTP.MCPAuth.CodeTTL) * time.Second
	}
	return 60 * time.Second
}

// ---- endpoints ----

// MCPOAuthServerMetadata serves RFC 8414 Authorization Server Metadata.
func (rt *Router) MCPOAuthServerMetadata(c *gin.Context) {
	if !rt.mcpAuthEnabled() {
		c.JSON(http.StatusNotFound, gin.H{"error": "mcp oauth authorization server is not enabled"})
		return
	}
	iss := rt.mcpIssuer(c)
	c.JSON(http.StatusOK, gin.H{
		"issuer":                                iss,
		"authorization_endpoint":                iss + mcpAuthorizePath,
		"token_endpoint":                        iss + mcpTokenPath,
		"registration_endpoint":                 iss + mcpRegisterPath,
		"revocation_endpoint":                   iss + mcpRevokePath,
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      []string{mcpScope},
	})
}

// MCPOAuthRegister implements RFC 7591 Dynamic Client Registration. The returned
// client_id is itself a signed JWT carrying the client metadata — no storage, so
// any instance can validate it.
func (rt *Router) MCPOAuthRegister(c *gin.Context) {
	if !rt.mcpAuthEnabled() {
		mcpOAuthErr(c, http.StatusNotFound, "invalid_request", "authorization server not enabled")
		return
	}
	var req struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		mcpOAuthErr(c, http.StatusBadRequest, "invalid_client_metadata", "invalid request body")
		return
	}
	if len(req.RedirectURIs) == 0 {
		mcpOAuthErr(c, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uris required")
		return
	}
	if len(req.RedirectURIs) > mcpMaxRedirectURIs {
		mcpOAuthErr(c, http.StatusBadRequest, "invalid_redirect_uri", "too many redirect_uris")
		return
	}
	for _, ru := range req.RedirectURIs {
		u, err := url.Parse(ru)
		if err != nil || !u.IsAbs() {
			mcpOAuthErr(c, http.StatusBadRequest, "invalid_redirect_uri", "invalid redirect_uri")
			return
		}
		// DCR is public, so scheme must be constrained: the frontend navigates the
		// browser to redirect_uri, and a javascript:/data: URI would execute in
		// n9e's own origin (session-token theft). Allow only http/https.
		if u.Scheme != "https" && u.Scheme != "http" {
			mcpOAuthErr(c, http.StatusBadRequest, "invalid_redirect_uri", "unsupported redirect_uri scheme")
			return
		}
	}
	if len(req.ClientName) > mcpMaxClientNameLen {
		req.ClientName = req.ClientName[:mcpMaxClientNameLen]
	}
	now := time.Now().Unix()
	clientID, err := rt.mcpSign(jwt.MapClaims{
		"token_use":     mcpUseClient,
		"client_name":   req.ClientName,
		"redirect_uris": req.RedirectURIs,
		"iat":           now,
	})
	if err != nil {
		mcpOAuthErr(c, http.StatusInternalServerError, "server_error", "")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusCreated, gin.H{
		"client_id":                  clientID,
		"client_id_issued_at":        now,
		"client_name":                req.ClientName,
		"redirect_uris":              req.RedirectURIs,
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	})
}

// MCPOAuthAuthorize validates the authorization request and 302-redirects the
// browser to the frontend consent route with a signed request ticket. The
// ticket carries the validated parameters tamper-proof across the GET → consent
// → decision-API hops, so no server-side session is needed (multi-instance safe,
// CSRF-safe by construction).
func (rt *Router) MCPOAuthAuthorize(c *gin.Context) {
	if !rt.mcpAuthEnabled() {
		mcpOAuthErr(c, http.StatusNotFound, "invalid_request", "authorization server not enabled")
		return
	}
	clientID := c.Query("client_id")
	redirectURI := c.Query("redirect_uri")

	client, err := rt.mcpParse(clientID, mcpUseClient)
	if err != nil {
		mcpOAuthErr(c, http.StatusBadRequest, "unauthorized_client", "unknown client")
		return
	}
	if !mcpRedirectAllowed(client, redirectURI) {
		// redirect_uri unvalidated → must NOT redirect to it (open-redirect guard).
		mcpOAuthErr(c, http.StatusBadRequest, "invalid_request", "redirect_uri mismatch")
		return
	}
	// From here redirect_uri is trusted; surface errors via redirect per OAuth.
	state := c.Query("state")
	if c.Query("response_type") != "code" {
		rt.mcpRedirectErr(c, redirectURI, state, "unsupported_response_type", "")
		return
	}
	codeChallenge := c.Query("code_challenge")
	if codeChallenge == "" || c.Query("code_challenge_method") != "S256" {
		rt.mcpRedirectErr(c, redirectURI, state, "invalid_request", "PKCE S256 required")
		return
	}
	resource := c.Query("resource")
	if resource == "" {
		resource = rt.mcpResource(c)
	}
	ticket, err := rt.mcpSign(jwt.MapClaims{
		"token_use":      mcpUseAuthzReq,
		"client_id":      clientID,
		"client_name":    mcpClaimString(client, "client_name"),
		"redirect_uri":   redirectURI,
		"code_challenge": codeChallenge,
		"state":          state,
		"scope":          c.Query("scope"),
		"resource":       resource,
		"exp":            time.Now().Add(mcpAuthzReqTTL).Unix(),
	})
	if err != nil {
		rt.mcpRedirectErr(c, redirectURI, state, "server_error", "")
		return
	}
	c.Redirect(http.StatusFound, rt.mcpBaseURL(c)+mcpConsentPath+"?req="+url.QueryEscape(ticket))
}

// MCPOAuthDecision is the protected (auth()+user()) endpoint the SPA consent page
// calls. It verifies the signed authorization-request ticket and the session
// user, then mints the authorization code bound to that user. Returns the
// redirect URL (n9e envelope) for the SPA to navigate to.
func (rt *Router) MCPOAuthDecision(c *gin.Context) {
	if !rt.mcpAuthEnabled() {
		ginx.Bomb(http.StatusNotFound, "mcp oauth authorization server is not enabled")
	}
	var body struct {
		Req      string `json:"req"`
		Decision string `json:"decision"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		ginx.Bomb(http.StatusBadRequest, "invalid request body")
	}
	ticket, err := rt.mcpParse(body.Req, mcpUseAuthzReq)
	if err != nil {
		ginx.Bomb(http.StatusBadRequest, "invalid or expired authorization request")
	}
	user := c.MustGet("user").(*models.User)
	redirectURI := mcpClaimString(ticket, "redirect_uri")
	state := mcpClaimString(ticket, "state")

	if body.Decision != "allow" {
		ginx.NewRender(c).Data(gin.H{"redirect": mcpRedirectErrURL(redirectURI, state, "access_denied", "")}, nil)
		return
	}

	code, err := rt.mcpSign(jwt.MapClaims{
		"token_use":      mcpUseCode,
		"sub":            strconv.FormatInt(user.Id, 10),
		"usr":            user.Username,
		"client_id":      mcpClaimString(ticket, "client_id"),
		"redirect_uri":   redirectURI,
		"code_challenge": mcpClaimString(ticket, "code_challenge"),
		"scope":          mcpClaimString(ticket, "scope"),
		"resource":       mcpClaimString(ticket, "resource"),
		"jti":            uuid.NewString(),
		"exp":            time.Now().Add(rt.mcpCodeTTL()).Unix(),
	})
	if err != nil {
		ginx.Bomb(http.StatusInternalServerError, "failed to issue authorization code")
	}
	u, err := url.Parse(redirectURI)
	if err != nil {
		ginx.Bomb(http.StatusBadRequest, "invalid redirect_uri")
	}
	q := u.Query()
	q.Set("code", code)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	ginx.NewRender(c).Data(gin.H{"redirect": u.String()}, nil)
}

// MCPOAuthToken implements the token endpoint for the authorization_code and
// refresh_token grants (public client + PKCE).
func (rt *Router) MCPOAuthToken(c *gin.Context) {
	if !rt.mcpAuthEnabled() {
		mcpOAuthErr(c, http.StatusNotFound, "invalid_request", "authorization server not enabled")
		return
	}
	switch c.PostForm("grant_type") {
	case "authorization_code":
		rt.mcpExchangeCode(c)
	case "refresh_token":
		rt.mcpExchangeRefresh(c)
	default:
		mcpOAuthErr(c, http.StatusBadRequest, "unsupported_grant_type", "")
	}
}

func (rt *Router) mcpExchangeCode(c *gin.Context) {
	code, err := rt.mcpParse(c.PostForm("code"), mcpUseCode)
	if err != nil {
		mcpOAuthErr(c, http.StatusBadRequest, "invalid_grant", "invalid code")
		return
	}
	if cid := c.PostForm("client_id"); cid != "" && mcpClaimString(code, "client_id") != cid {
		mcpOAuthErr(c, http.StatusBadRequest, "invalid_grant", "client_id mismatch")
		return
	}
	if mcpClaimString(code, "redirect_uri") != c.PostForm("redirect_uri") {
		mcpOAuthErr(c, http.StatusBadRequest, "invalid_grant", "redirect_uri mismatch")
		return
	}
	if !mcpVerifyPKCE(c.PostForm("code_verifier"), mcpClaimString(code, "code_challenge")) {
		mcpOAuthErr(c, http.StatusBadRequest, "invalid_grant", "PKCE verification failed")
		return
	}
	resource := mcpClaimString(code, "resource")
	if r := c.PostForm("resource"); r != "" && r != resource {
		mcpOAuthErr(c, http.StatusBadRequest, "invalid_target", "resource mismatch")
		return
	}
	jti := mcpClaimString(code, "jti")
	if jti == "" {
		mcpOAuthErr(c, http.StatusBadRequest, "invalid_grant", "invalid code")
		return
	}
	// One-time-use guard — the only shared state. SetNX is atomic across all
	// instances on the shared Redis, so a replayed code (even raced between two
	// instances) is rejected exactly once. TTL = code TTL; after it the code JWT
	// is expired anyway and fails the signature/exp check first.
	ok, err := rt.Redis.SetNX(c.Request.Context(), mcpCodeRedisPrefix+jti, "1", rt.mcpCodeTTL()).Result()
	if err != nil {
		mcpOAuthErr(c, http.StatusInternalServerError, "server_error", "code store unavailable")
		return
	}
	if !ok {
		mcpOAuthErr(c, http.StatusBadRequest, "invalid_grant", "authorization code already used")
		return
	}
	rt.mcpIssueTokenPair(c, mcpClaimString(code, "sub"), mcpClaimString(code, "usr"),
		mcpClaimString(code, "client_id"), resource, mcpClaimString(code, "scope"))
}

func (rt *Router) mcpExchangeRefresh(c *gin.Context) {
	claims, err := rt.mcpParse(c.PostForm("refresh_token"), mcpUseRefresh)
	if err != nil {
		mcpOAuthErr(c, http.StatusBadRequest, "invalid_grant", "invalid refresh_token")
		return
	}
	if cid := c.PostForm("client_id"); cid != "" && mcpClaimString(claims, "client_id") != cid {
		mcpOAuthErr(c, http.StatusBadRequest, "invalid_grant", "client_id mismatch")
		return
	}
	rt.mcpIssueTokenPair(c, mcpClaimString(claims, "sub"), mcpClaimString(claims, "usr"),
		mcpClaimString(claims, "client_id"), mcpClaimString(claims, "aud"), mcpClaimString(claims, "scope"))
}

func (rt *Router) mcpIssueTokenPair(c *gin.Context, sub, usr, clientID, resource, scope string) {
	now := time.Now()
	access, err := rt.mcpSign(jwt.MapClaims{
		"token_use": mcpUseAccess,
		"iss":       rt.mcpIssuer(c),
		"sub":       sub,
		"usr":       usr,
		"aud":       resource,
		"client_id": clientID,
		"scope":     scope,
		"jti":       uuid.NewString(),
		"exp":       now.Add(rt.mcpAccessTTL()).Unix(),
	})
	if err != nil {
		mcpOAuthErr(c, http.StatusInternalServerError, "server_error", "")
		return
	}
	refresh, err := rt.mcpSign(jwt.MapClaims{
		"token_use": mcpUseRefresh,
		"sub":       sub,
		"usr":       usr,
		"aud":       resource,
		"client_id": clientID,
		"scope":     scope,
		"jti":       uuid.NewString(),
		"exp":       now.Add(rt.mcpRefreshTTL()).Unix(),
	})
	if err != nil {
		mcpOAuthErr(c, http.StatusInternalServerError, "server_error", "")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"access_token":  access,
		"token_type":    "Bearer",
		"expires_in":    int(rt.mcpAccessTTL().Seconds()),
		"refresh_token": refresh,
		"scope":         scope,
	})
}

// MCPOAuthRevoke implements RFC 7009. Tokens are stateless (no rotation/denylist
// in this build), so there is nothing to delete; per §2.2 respond 200 regardless.
func (rt *Router) MCPOAuthRevoke(c *gin.Context) {
	c.Status(http.StatusOK)
}

// ---- helpers ----

func mcpVerifyPKCE(verifier, challenge string) bool {
	if verifier == "" || challenge == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	got := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(got), []byte(challenge)) == 1
}

func mcpRedirectAllowed(client jwt.MapClaims, given string) bool {
	if given == "" {
		return false
	}
	for _, u := range mcpClaimStrings(client, "redirect_uris") {
		if u == given {
			return true
		}
	}
	return false
}

func (rt *Router) mcpRedirectErr(c *gin.Context, redirectURI, state, code, desc string) {
	if redirectURI == "" {
		mcpOAuthErr(c, http.StatusBadRequest, code, desc)
		return
	}
	c.Redirect(http.StatusFound, mcpRedirectErrURL(redirectURI, state, code, desc))
}

func mcpRedirectErrURL(redirectURI, state, code, desc string) string {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return redirectURI
	}
	q := u.Query()
	q.Set("error", code)
	if desc != "" {
		q.Set("error_description", desc)
	}
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func mcpOAuthErr(c *gin.Context, status int, code, desc string) {
	c.Header("Cache-Control", "no-store")
	body := gin.H{"error": code}
	if desc != "" {
		body["error_description"] = desc
	}
	c.JSON(status, body)
}

func mcpClaimString(m jwt.MapClaims, k string) string {
	s, _ := m[k].(string)
	return s
}

func mcpClaimStrings(m jwt.MapClaims, k string) []string {
	raw, ok := m[k].([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
