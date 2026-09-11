# Built-in OAuth 2.1 Authorization Server for A2A / MCP

This makes n9e **an OAuth 2.1 authorization server in its own right** (co-located with the resource server), so that generic MCP clients
(Claude, ChatGPT connectors) can reach `/a2a` and `/mcp` with **zero configuration** via RFC 7591 Dynamic Client Registration (DCR).
It **complements** [a2a-oauth-rs.md](./a2a-oauth-rs.md) (n9e as a resource server in front of an external enterprise IdP) and targets
the "**self-hosted, no external IdP**" scenario. It is disabled by default and sits **alongside** the other three authentication paths —
`X-User-Token` (PAT), self-signed session JWT, and external-IdP RS — without interfering with any of them when enabled.

## 1. What it solves, and how it relates to RSAuth

- **RSAuth (existing)**: n9e acts as a resource server and accepts tokens issued by an **external enterprise IdP** — a good fit when you already run Keycloak or Entra.
- **MCPAuth (this document)**: n9e acts as its own authorization server, issuing its own tokens and handling DCR itself — a good fit when you have no IdP and want Claude or ChatGPT to connect directly.
- The two **can be enabled at the same time**: the `authorization_servers` field of `/.well-known/oauth-protected-resource` will **list both** n9e itself and the external IdP, and each client picks what it needs (generic clients go through n9e's DCR, enterprise clients go through the IdP).

## 2. Design highlights (streamlined v2)

- **Stateless first**: client_id, authorization-request tickets, authorization codes, access tokens, and refresh tokens are **all HS256-signed JWTs**, distinguished by a `token_use` claim. The signing key is **derived via HKDF-SHA256** from `JWTAuth.SigningKey` (cryptographically isolated from the session key — an MCP token cannot be used as a session token, and vice versa).
- **The only shared state**: authorization codes are single-use — redeeming one performs a single Redis `SetNX` on the code's `jti`, and a replay is rejected. This is **atomic and safe across instances** (multiple n9e center instances share the same Redis).
- **No refresh rotation**: refresh tokens in this version are stateless with no reuse detection; revocation relies on a short TTL or on rotating the signing key. If you need public-internet, multi-tenant grade security, refresh rotation can be added later.
- **Consent lives in the frontend**: n9e sessions are header/Bearer based with no cookies, so after validation `/oauth/authorize` **302-redirects to the frontend SPA route `/oauth-consent`**. The SPA (which holds the token and supports SSO login) shows the consent page and then calls the protected decision API to issue the authorization code.

## 3. Protocol compliance

Implements OAuth 2.1 + RFC 8414 (AS metadata) + RFC 9728 (protected resource metadata) + RFC 7591 (DCR) + RFC 7636 (PKCE, **S256 enforced**) + RFC 8707 (resource-bound `aud`) + RFC 7009 (revocation). Authorization codes are strictly single-use; `redirect_uri` is matched exactly to prevent open redirects; the access token's `aud` is bound to the resource and a mismatch is rejected at validation time (preventing token passthrough).

## 4. Configuration in `etc/config.toml`

```toml
[HTTP.MCPAuth]
Enable = true
# The canonical URL of this AS (the `iss` of issued tokens and the `issuer` in RFC 8414 metadata).
# With multiple instances you must set it explicitly so every instance advertises the same value;
# if left empty it is derived from the request Host plus X-Forwarded-Proto (single-node only)
Issuer = "https://n9e.example.com"
# The MCP resource identifier, bound into the access token's aud (RFC 8707). If empty it falls back
# to RSAuth.Audience, then to "<base>/mcp". When RSAuth is also enabled, setting it to the same value
# as RSAuth.Audience is recommended
Resource = "https://n9e.example.com/mcp"
# If empty, derived from JWTAuth.SigningKey via HKDF (recommended). With multiple instances it must be
# identical on every instance — never generate it randomly per process
# SigningKey = ""
# Lifetimes in seconds; leave at 0 for the defaults: access 3600 / refresh 604800 / code 60
# AccessTTL = 3600
# RefreshTTL = 604800
# CodeTTL = 60
```

| Field | Default | Description |
|---|---|---|
| Enable | false | Master switch for the built-in AS. When off, every `/oauth/*` endpoint returns 404 and behavior is unchanged |
| Issuer | "" | Canonical URL of the AS. **Required with multiple instances**, otherwise each instance derives its own value from the request and they may diverge |
| Resource | "" | The access token's `aud`; empty → RSAuth.Audience → `<base>/mcp` |
| SigningKey | "" | If empty, derived from `JWTAuth.SigningKey` via HKDF (isolated from sessions, identical across instances) |
| AccessTTL/RefreshTTL/CodeTTL | 3600/604800/60 | Seconds |

> ⚠️ **Multi-instance constraints**: (1) `SigningKey` (or the `JWTAuth.SigningKey` it is derived from) must be **byte-for-byte identical on every instance** — never generate it randomly per process, or tokens will only validate on the instance that issued them. (2) With multiple instances, **set `Issuer`/`Resource` explicitly** instead of relying on request-based derivation. (3) Apart from the single-use authorization code (shared Redis), everything is stateless, so **never use an in-process cache** to store codes or tickets.

## 5. Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/.well-known/oauth-authorization-server` (plus `/a2a` and `/mcp` aliases) | Public | RFC 8414 AS metadata |
| GET | `/.well-known/oauth-protected-resource` (plus aliases) | Public | RFC 9728; `authorization_servers` includes n9e itself |
| POST | `/oauth/register` | Public | RFC 7591 DCR, returns a signed client_id |
| GET | `/oauth/authorize` | Public | Validates parameters → 302 to the frontend at `/oauth-consent?req=<ticket>` |
| POST | `/api/n9e/mcp/oauth/authorize` | **session** | Decision API: after the user consents, the frontend POSTs `{req, decision}` → an authorization code is issued → returns `{redirect}` |
| POST | `/oauth/token` | public client + PKCE | `authorization_code` (single-use + PKCE + resource check) / `refresh_token` |
| POST | `/oauth/revoke` | public client | RFC 7009; tokens are stateless, so this is best-effort and always returns 200 |

## 6. Authorization flow

1. An MCP client calls `/mcp` without a token → `401` + `WWW-Authenticate: Bearer resource_metadata=…`.
2. The client fetches `/.well-known/oauth-protected-resource` → finds AS = n9e → fetches `/.well-known/oauth-authorization-server` → `POST /oauth/register` (DCR) → jumps to `/oauth/authorize?...&code_challenge=...&resource=<base>/mcp`.
3. n9e validates client_id (signature), redirect_uri (exact match), `response_type=code`, PKCE S256, and resource → signs an authorization-request ticket → **302 to `/oauth-consent?req=<ticket>`** (a frontend SPA route).
4. Frontend SPA: if not logged in, it redirects to `/login?redirect=...` (SSO included); if logged in, it shows the consent page → the user clicks "Allow" → `POST /api/n9e/mcp/oauth/authorize {req, decision:"allow"}` (with the session) → the backend validates the session and the ticket → signs an **authorization code** (bound to that user) → returns `{redirect}` → the SPA does `window.location` back to `redirect_uri?code=...&state=...`.
5. The client calls `POST /oauth/token` (code + code_verifier + resource) → PKCE check + **single-use SetNX** + matching resource → issues an access token (and a refresh token).
6. The client calls `/mcp` with `Authorization: Bearer <access>` → n9e validates the signature with the MCP key and lets it through (mapped to the corresponding user, with the same permissions as that user inside the agent). Note that this access token is **only accepted on the `/a2a` and `/mcp` endpoints** and cannot be used against other `/api/n9e/*` APIs (see `agentOAuthScope` in router_mw.go).

## 7. Self-test with curl

```bash
BASE=http://127.0.0.1:17000

# 1) DCR to obtain a client_id
CID=$(curl -s -XPOST $BASE/oauth/register -H 'Content-Type: application/json' \
  -d '{"client_name":"cli","redirect_uris":["http://127.0.0.1:9999/cb"]}' | jq -r .client_id)

# 2) Build the PKCE pair
VERIFIER=$(openssl rand -hex 32)
CHALLENGE=$(printf %s "$VERIFIER" | openssl dgst -sha256 -binary | openssl base64 | tr '+/' '-_' | tr -d '=')

# 3) Open the authorize URL in a browser (it redirects to the frontend /oauth-consent for login
#    and consent, then returns the code on the callback)
echo "$BASE/oauth/authorize?response_type=code&client_id=$CID&redirect_uri=http://127.0.0.1:9999/cb&code_challenge=$CHALLENGE&code_challenge_method=S256&state=xyz"

# 4) Exchange the code for a token (take CODE from the callback URL)
curl -s -XPOST $BASE/oauth/token -d grant_type=authorization_code -d code=$CODE \
  -d code_verifier=$VERIFIER -d redirect_uri=http://127.0.0.1:9999/cb -d client_id=$CID | jq

# 5) Call MCP with the access_token
curl -s -XPOST $BASE/mcp -H "Authorization: Bearer $ACCESS" -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'

# Counter-example: replay the same code from step 4 → {"error":"invalid_grant", ...} (single-use guard)
```

## 8. Code involved

- `pkg/httpx/httpx.go` — the `MCPAuth` configuration
- `center/router/router_mcp_oauth.go` — all AS endpoints, the decision API, and the JWT/PKCE/HKDF/single-use-code helpers
- `center/router/router_rsauth.go` — the discovery-chain switch broadened to `rsAuthEnabled() || mcpAuthEnabled()`; the `authorization_servers` of `oauthProtectedResource` now includes n9e itself
- `center/router/router_mw.go` — the builtin branch of `tokenAuth()` (MCP key signature validation, ordered ahead of the external-IdP RS); `agentOAuthScope` (restricts OAuth acceptance to `/a2a` and `/mcp`)
- `center/router/router_a2a.go` / `router.go` — endpoint registration (public `/oauth/*` at the root, decision API under `/api/n9e`)
- Frontend `n9e/fe`: `src/pages/oauthConsent` plus the `/oauth-consent` route in `src/routers/index.tsx`
