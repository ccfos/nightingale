package router

import (
	"context"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/golang-jwt/jwt"
	"github.com/toolkits/pkg/logger"
)

// rsAuthEnabled reports whether OAuth 2.1 Resource Server authentication is
// usable: the switch is on, an audience is configured (binding tokens to this
// service is mandatory), and the OIDC login provider it borrows for JWKS
// verification is available.
func (rt *Router) rsAuthEnabled() bool {
	return rt.HTTP.RSAuth.Enable && rt.HTTP.RSAuth.Audience != "" &&
		rt.Sso != nil && rt.Sso.OIDC != nil && rt.Sso.OIDC.Enable
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

// authByIdPAccessToken verifies an external IdP access token and resolves it to
// a local user, provisioning one on first sight with the same defaults as OIDC
// login so an agent-token user is indistinguishable from one created by an
// interactive SSO login.
func (rt *Router) authByIdPAccessToken(ctx context.Context, rawToken string) (*models.User, error) {
	out, err := rt.Sso.OIDC.VerifyAccessToken(ctx, rawToken, rt.HTTP.RSAuth.Audience)
	if err != nil {
		return nil, err
	}

	user, err := models.UserGetByUsername(rt.Ctx, out.Username)
	if err != nil {
		return nil, err
	}
	if user != nil {
		return user, nil
	}

	user = new(models.User)
	user.FullSsoFields("oidc", out.Username, out.Nickname, out.Phone, out.Email, rt.Sso.OIDC.DefaultRoles)
	if err := user.Add(rt.Ctx); err != nil {
		// A concurrent request may have just created the same user; fall back to
		// the existing row instead of failing this call.
		if existing, getErr := models.UserGetByUsername(rt.Ctx, out.Username); getErr == nil && existing != nil {
			return existing, nil
		}
		return nil, err
	}

	for _, gid := range rt.Sso.OIDC.DefaultTeams {
		if err := models.UserGroupMemberAdd(rt.Ctx, gid, user.Id); err != nil {
			logger.Warningf("[RS] add user %s to group %d failed: %v", user.Username, gid, err)
		}
	}

	return user, nil
}
