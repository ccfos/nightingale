package skillgateway

import "strings"

// defaultDenyN9eAPIPaths is the built-in blacklist: GET paths (under /api/n9e)
// that return secrets or aren't safe reads. Matched as case-insensitive PREFIXES
// (so "/datasource" also covers "/datasources" and "/datasource/:id"), erring
// toward over-blocking. This list is the security floor — Deny.N9eAPI only ADDS
// to it, never removes — so keep it comprehensive: in the deny-list model a read
// endpoint that's NOT here is reachable.
//
// Note the trailing slash on "/user/" — it blocks "/user/:id" without catching
// the safe "/user-groups" (teams).
var defaultDenyN9eAPIPaths = []string{
	"/datasource",     // datasource configs carry Settings/Auth (DB passwords, API keys)
	"/notify-channel", // notify-channel configs carry app secrets / SMTP passwords / tokens
	"/notify-config",  // global notify config (notifyConfigGet): SMTP host/user/PASS, IM tokens
	"/config",         // config-center KV (configGetByKey): arbitrary stored secrets incl. smtp
	"/user-variable",  // user variables: plaintext-type (encrypted=0) values returned verbatim
	"/users",          // user list: password hashes + contacts (notify tokens)
	"/user/",          // single user detail
	"/self/token",     // the caller's own API tokens
	"/user-token",     // user token management
	"/sso",            // SSO / identity-provider configs (client secrets)
	"/ldap",
	"/oidc",
	"/oauth",
	"/cas",
	"/proxy/",   // datasource proxy — reaches the datasource WITH its credentials
	"/webhook",  // generic webhook configs (may embed tokens)
	"/password", // any password-change/reset endpoints
}

// mergeDenyPaths combines the built-in blacklist with operator-supplied extras
// (Deny.N9eAPI), normalizing each to a lowercase, leading-slash prefix.
func mergeDenyPaths(extra []string) []string {
	out := make([]string, 0, len(defaultDenyN9eAPIPaths)+len(extra))
	out = append(out, defaultDenyN9eAPIPaths...)
	for _, e := range extra {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" {
			continue
		}
		if !strings.HasPrefix(e, "/") {
			e = "/" + e
		}
		out = append(out, e)
	}
	return out
}
