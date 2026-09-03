package router

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
	"github.com/toolkits/pkg/errorx"
	"github.com/toolkits/pkg/logger"
)

// rsAuthProvider returns the configured Resource Server token provider,
// defaulting to "oidc" when unset.
func (rt *Router) rsAuthProvider() string {
	if rt.HTTP.RSAuth.Provider == "oauth2" {
		return "oauth2"
	}
	return "oidc"
}

// rsAuthEnabled reports whether OAuth 2.1 Resource Server authentication is
// usable: the switch is on, an audience is configured (binding tokens to this
// service is mandatory), and the selected login provider it borrows for token
// verification is available. The OIDC provider verifies JWTs via JWKS; the
// OAuth2 provider verifies opaque tokens via its RFC 7662 introspection
// endpoint.
func (rt *Router) rsAuthEnabled() bool {
	if !rt.HTTP.RSAuth.Enable || rt.HTTP.RSAuth.Audience == "" || rt.Sso == nil {
		return false
	}
	switch rt.rsAuthProvider() {
	case "oauth2":
		if rt.Sso.OAuth2 == nil || !rt.Sso.OAuth2.Enable {
			return false
		}
		if rt.Sso.OAuth2.RSVerifyMethod == "introspect" {
			return rt.Sso.OAuth2.IntrospectAddr != ""
		}
		return rt.Sso.OAuth2.UserInfoAddr != ""
	default:
		return rt.Sso.OIDC != nil && rt.Sso.OIDC.Enable
	}
}

// oidcDiscoveryURL returns the trusted IdP's OpenID Connect discovery URL to
// advertise in the AgentCard, or "" when RS auth is inactive or the provider is
// not OIDC (a plain OAuth2 IdP has no OIDC discovery document, so the AgentCard
// only advertises the x-user-token scheme for it).
func (rt *Router) oidcDiscoveryURL() string {
	if !rt.rsAuthEnabled() || rt.rsAuthProvider() != "oidc" {
		return ""
	}
	return strings.TrimSuffix(rt.Sso.OIDC.SsoAddr, "/") + "/.well-known/openid-configuration"
}

// rsAuthServerAddr returns the trusted authorization server identifier for the
// active RS provider, used in the RFC 9728 resource metadata. Callers must
// ensure rsAuthEnabled() first.
func (rt *Router) rsAuthServerAddr() string {
	if rt.rsAuthProvider() == "oauth2" {
		return rt.Sso.OAuth2.SsoAddr
	}
	return rt.Sso.OIDC.SsoAddr
}

// oauthProtectedResource serves the RFC 9728 Protected Resource Metadata so
// OAuth-aware clients can discover which authorization server(s) issue tokens for
// this service and which audience to request. It returns 404 while neither the
// external-IdP RS nor the built-in AS is enabled, so a disabled feature
// advertises nothing.
func (rt *Router) oauthProtectedResource(c *gin.Context) {
	rsOn := rt.rsAuthEnabled()
	mcpOn := rt.mcpAuthEnabled()
	if !rsOn && !mcpOn {
		c.JSON(http.StatusNotFound, gin.H{"error": "oauth is not enabled"})
		return
	}
	// authorization_servers lists every AS that can mint tokens for this
	// resource: the built-in AS (n9e itself) and/or the trusted external IdP.
	// A client picks one it can use — generic MCP clients need the built-in AS's
	// Dynamic Client Registration, enterprise clients use their IdP.
	var servers []string
	if mcpOn {
		servers = append(servers, rt.mcpIssuer(c))
	}
	if rsOn {
		servers = append(servers, rt.rsAuthServerAddr())
	}
	resource := strings.TrimSpace(rt.HTTP.RSAuth.Audience)
	if resource == "" && mcpOn {
		resource = rt.mcpResource(c)
	}
	c.JSON(http.StatusOK, gin.H{
		"resource":                 resource,
		"authorization_servers":    servers,
		"bearer_methods_supported": []string{"header"},
	})
}

// protectedResourceMetadataURL builds the absolute URL of this service's RFC
// 9728 Protected Resource Metadata document, used as the `resource_metadata`
// pointer in the 401 WWW-Authenticate challenge. It prefers the configured A2A
// BaseURL and otherwise derives scheme+host from the request (honouring a
// reverse proxy's X-Forwarded-Proto), mirroring how the AgentCard derives its
// own base so the two discovery surfaces always agree.
func (rt *Router) protectedResourceMetadataURL(c *gin.Context) string {
	base := strings.TrimSuffix(rt.HTTP.A2A.BaseURL, "/")
	if base == "" {
		scheme := "http"
		if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		base = scheme + "://" + c.Request.Host
	}
	return base + "/.well-known/oauth-protected-resource"
}

// wwwAuthenticateChallenge builds the RFC 6750 / RFC 9728 `WWW-Authenticate`
// header advertised on 401s from the OAuth-protected agent endpoints. The
// `resource_metadata` parameter is the discovery entry point an MCP client
// follows to learn the trusted authorization server with zero manual config;
// `error="invalid_token"` is appended only when a Bearer token was actually
// presented and rejected (vs. simply absent), per RFC 6750 §3.1.
func (rt *Router) wwwAuthenticateChallenge(c *gin.Context) string {
	challenge := fmt.Sprintf("Bearer resource_metadata=%q", rt.protectedResourceMetadataURL(c))
	if rt.extractToken(c.Request) != "" {
		challenge += `, error="invalid_token"`
	}
	return challenge
}

// rsAuthChallenge completes the OAuth discovery entry point by attaching the
// WWW-Authenticate hint to 401 responses from the a2a/mcp endpoints while
// Resource Server auth is active — the missing third leg alongside the
// protected-resource metadata endpoint and the AgentCard oidc scheme. An MCP
// client (ChatGPT/Claude connector) that calls without a token gets the 401 +
// pointer and can self-discover the IdP; without it those clients require
// manual IdP/audience configuration.
//
// It is scoped to the agent groups on purpose: tokenAuth is shared with the
// rest of the API, and the browser session-JWT login flow must keep getting a
// plain 401 with no OAuth challenge. Downstream auth middlewares
// (tokenAuth/user) report failure by panicking with errorx.PageError via
// ginx.Bomb; this recovers a 401, sets the header on the still-unwritten
// response, and re-panics so the global aop.Recovery renders the body as
// before. Non-401 panics are re-raised untouched.
func (rt *Router) rsAuthChallenge() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rt.rsAuthEnabled() && !rt.mcpAuthEnabled() {
			c.Next()
			return
		}
		defer func() {
			if r := recover(); r != nil {
				if e, ok := r.(errorx.PageError); ok && e.Code == http.StatusUnauthorized {
					c.Header("WWW-Authenticate", rt.wwwAuthenticateChallenge(c))
				}
				panic(r)
			}
		}()
		c.Next()
	}
}

// tokenHasIssuer reports whether a JWT carries a non-empty `iss` claim, without
// verifying the signature. n9e's own session tokens (createTokens) set no
// issuer, so a present `iss` is what tells an external IdP access token — which
// must go through Resource Server verification — apart from a session JWT, and
// keeps the existing self-signed JWT path untouched.
func tokenHasIssuer(raw string) bool {
	claims := jwt.MapClaims{}
	if _, _, err := new(jwt.Parser).ParseUnverified(raw, claims); err != nil {
		return false
	}
	iss, _ := claims["iss"].(string)
	return iss != ""
}

// isJWT reports whether raw is structurally a JWT (three dot-separated base64url
// segments). Opaque OAuth2 access tokens and n9e's x-user-token are not JWTs and
// return false.
func isJWT(raw string) bool {
	_, _, err := new(jwt.Parser).ParseUnverified(raw, jwt.MapClaims{})
	return err == nil
}

// shouldVerifyAsRS decides whether a bearer token should be sent through
// Resource Server verification. With the OIDC provider, external tokens are
// JWTs carrying an `iss`, while n9e's own session JWT has none — so the issuer
// is the discriminator. With the OAuth2 provider, external access tokens are
// opaque; n9e's own opaque x-user-token arrives in a separate header
// (X-User-Token) and is handled before this step, while extractToken only reads
// the Authorization: Bearer header — so any non-JWT bearer here is an external
// token to verify. n9e's session JWT (a JWT) is never sent to the IdP.
func (rt *Router) shouldVerifyAsRS(raw string) bool {
	if rt.rsAuthProvider() == "oauth2" {
		return !isJWT(raw)
	}
	return tokenHasIssuer(raw)
}

// VerifyAgentOAuthToken verifies an agent-plane OAuth access token — builtin
// AS first (local signature), then the external IdP Resource Server path —
// and resolves the local user identity, provisioning a user on first sight
// for IdP tokens exactly as tokenAuth's inline branches do.
//
// It exists for embedder routers (e.g. the enterprise edition): their own
// tokenAuth copies know nothing about OAuth — the builtin-AS keys and IdP
// config live on this Router — so for /mcp in-process dispatches (marked via
// a2a.IsMCPInProcDispatch) they delegate verification here instead of
// duplicating the crypto/IdP logic. Returns ok=false when neither auth mode
// is enabled or the token fails verification.
func (rt *Router) VerifyAgentOAuthToken(ctx context.Context, rawToken string) (userid int64, username string, ok bool) {
	if rawToken == "" {
		return 0, "", false
	}
	if rt.mcpAuthEnabled() {
		if uid, uname, ok := rt.mcpVerifyAccessToken(rawToken); ok {
			return uid, uname, true
		}
	}
	if rt.rsAuthEnabled() && rt.shouldVerifyAsRS(rawToken) {
		user, err := rt.authByIdPAccessToken(ctx, rawToken)
		if err == nil && user != nil {
			return user.Id, user.Username, true
		}
	}
	return 0, "", false
}

// authByIdPAccessToken verifies an external IdP access token and resolves it to
// a local user, provisioning one on first sight with the same defaults as SSO
// login so an agent-token user is indistinguishable from one created by an
// interactive login. Verification is dispatched to the configured provider
// (OIDC JWKS or OAuth2 introspection); the provisioning path is shared.
func (rt *Router) authByIdPAccessToken(ctx context.Context, rawToken string) (*models.User, error) {
	var (
		username, nickname, phone, email string
		source                           string
		defaultRoles                     []string
		defaultTeams                     []int64
	)

	switch rt.rsAuthProvider() {
	case "oauth2":
		out, err := rt.Sso.OAuth2.VerifyAccessToken(ctx, rawToken, rt.HTTP.RSAuth.Audience)
		if err != nil {
			return nil, err
		}
		username, nickname, phone, email = out.Username, out.Nickname, out.Phone, out.Email
		source = "oauth2"
		defaultRoles = rt.Sso.OAuth2.DefaultRoles
	default:
		out, err := rt.Sso.OIDC.VerifyAccessToken(ctx, rawToken, rt.HTTP.RSAuth.Audience)
		if err != nil {
			return nil, err
		}
		username, nickname, phone, email = out.Username, out.Nickname, out.Phone, out.Email
		source = "oidc"
		defaultRoles = rt.Sso.OIDC.DefaultRoles
		defaultTeams = rt.Sso.OIDC.DefaultTeams
	}

	user, err := models.UserGetByUsername(rt.Ctx, username)
	if err != nil {
		return nil, err
	}
	if user != nil {
		return user, nil
	}

	user = new(models.User)
	user.FullSsoFields(source, username, nickname, phone, email, defaultRoles)
	if err := user.Add(rt.Ctx); err != nil {
		// A concurrent request may have just created the same user; fall back to
		// the existing row instead of failing this call.
		if existing, getErr := models.UserGetByUsername(rt.Ctx, username); getErr == nil && existing != nil {
			return existing, nil
		}
		return nil, err
	}

	for _, gid := range defaultTeams {
		if err := models.UserGroupMemberAdd(rt.Ctx, gid, user.Id); err != nil {
			logger.Warningf("[RS] add user %s to group %d failed: %v", user.Username, gid, err)
		}
	}

	return user, nil
}
