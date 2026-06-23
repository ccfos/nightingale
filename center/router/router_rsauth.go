package router

import (
	"context"
	"net/http"
	"strings"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
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
// OAuth-aware clients can discover which authorization server issues tokens for
// this service and which audience to request. It returns 404 while RS auth is
// inactive, so a disabled feature advertises nothing. (The 401 WWW-Authenticate
// discovery hint is intentionally not emitted yet — see doc/api/a2a-oauth-rs.md.)
func (rt *Router) oauthProtectedResource(c *gin.Context) {
	if !rt.rsAuthEnabled() {
		c.JSON(http.StatusNotFound, gin.H{"error": "oauth resource server is not enabled"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"resource":                 rt.HTTP.RSAuth.Audience,
		"authorization_servers":    []string{rt.rsAuthServerAddr()},
		"bearer_methods_supported": []string{"header"},
	})
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
