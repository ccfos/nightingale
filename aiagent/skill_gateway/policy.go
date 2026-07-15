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

// postAllowN9eAPIPaths is the allowlist of read-only DATA-query endpoints the
// gateway may forward with POST. They are POST only because they carry a JSON
// query body that is too large/structured for query params — not because they
// mutate state: each runs the query under the caller's RBAC (the handler's
// CheckDsPerm) and returns time-series / log DATA, never config or secrets. POST
// to any path NOT in this set is refused, so this is an allowlist, not the GET
// deny-list model. Matched as exact, normalized paths (relative to /api/n9e).
var postAllowN9eAPIPaths = map[string]bool{
	"/ds-query":            true, // unified datasource query (metrics/logs by cate)
	"/query-range-batch":   true, // Prometheus range query (batch)
	"/query-instant-batch": true, // Prometheus instant query (batch)
	"/logs-query":          true, // log query (v2)
	"/log-query":           true, // log query
	"/log-query-batch":     true, // log query (batch)
}

// getAllowExceptions are GET paths that must stay reachable even though they sit
// under a denied prefix. /datasource/brief is the SECRET-REDACTED datasource
// list (the handler runs RedactSecrets; it's what the UI dropdowns use) — skills
// need it to discover datasource ids/cates to query, but the broad "/datasource"
// deny prefix would otherwise block it. Matched as an exact, normalized path.
var getAllowExceptions = map[string]bool{
	"/datasource/brief": true,
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
