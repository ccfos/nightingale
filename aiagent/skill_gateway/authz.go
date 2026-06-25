// Package skillgateway is the in-process Skill Gateway (design §12): the broker a
// sandboxed skill script talks to (over a per-exec UNIX socket) when it needs an
// n9e internal API. It is n9e-specific — Claude Code has no equivalent — and
// reuses n9e's existing identity + RBAC rather than inventing a new model:
//
//   - Identity is BOUND at sandbox start (the chat session owner), never supplied
//     by the script (§12.1). The gateway resolves the *models.User once and every
//     request on that socket acts as exactly that user — no impersonation possible.
//   - Authorization = the platform's grantable_n9e_api envelope ∩ the user's own
//     RBAC (CheckPerm / busi-group scope) − the hard-deny list (§12.2). A skill
//     can therefore never exceed the user who launched it, nor the admin envelope.
//   - It calls models.* directly in-process with the resolved *User + DBCtx (no
//     token, no HTTP) — the same shape as the existing aiagent built-in tools
//     (aiagent/tools/common.go getUser+checkPerm). Credentials never enter the
//     sandbox (§12.4).
package skillgateway

import "strings"

// grantMatched reports whether a capability token (e.g. "alert:read") matches any
// pattern in pats. A pattern is "<domain>:<verb>" with "*" allowed on either
// segment, or the bare "*" meaning everything. It backs BOTH gates (§12.2/§16.3):
//   - the grantable envelope  (allow):  e.g. ["alert:read","datasource:read"]
//   - the hard-deny list      (deny):   e.g. ["*:write","*:delete","user:*"]
func grantMatched(pats []string, token string) bool {
	td, tv := splitGrant(token)
	for _, p := range pats {
		p = strings.TrimSpace(strings.ToLower(p))
		switch {
		case p == "":
			continue
		case p == "*":
			return true
		}
		pd, pv := splitGrant(p)
		if (pd == "*" || pd == td) && (pv == "*" || pv == tv) {
			return true
		}
	}
	return false
}

// splitGrant splits "domain:verb" (lowercased). A token with no ':' is treated as
// "<token>:" so a bare "user" deny pattern matches the "user" domain at any verb
// only when written "user:*"; plain "user" matches domain "user" + empty verb.
func splitGrant(s string) (domain, verb string) {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.IndexByte(s, ':'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}
