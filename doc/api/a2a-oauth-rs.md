# Connecting A2A / MCP to an Enterprise IdP (OAuth 2.1 Resource Server)

This makes n9e's a2a / mcp agent endpoints accept **OAuth access tokens issued by an external enterprise IdP (Keycloak, Entra ID, Okta, Auth0, and so on)**
as **per-user** credentials. When an agent orchestration platform calls n9e on behalf of a specific user, it carries that user's access token;
n9e validates the token and maps the call onto the corresponding local user — the audit trail names a real person, permissions match that user's,
and no shared robot identity is involved any more.

This mechanism sits **alongside** the existing `X-User-Token` (PAT) and self-signed session JWT: satisfying any one of them is enough. It is disabled by default and, once enabled, does not affect the two existing authentication paths.

## 1. How it works

- **Two providers** (`[HTTP.RSAuth].Provider`; each reuses the corresponding SSO login configuration, so you do not have to configure a second authorization server):
  - `oidc` (default): reuses the IdP referenced by the **OIDC login** configuration and validates JWT access tokens **locally** using its issuer and JWKS.
  - `oauth2`: reuses the IdP referenced by the **OAuth2 login** configuration and validates **opaque** access tokens (validation method described under `RSVerifyMethod` in section 2.4).
- **Validate only, never issue**: n9e acts purely as a resource server (validating tokens issued by someone else); the authorization server is always the external IdP.
- **How an OAuth token is recognized**: the request carries `Authorization: Bearer <token>`. A self-signed n9e session JWT **has no `iss`** and is itself a JWT, so:
  - with the `oidc` provider, only a JWT that **carries `iss`** goes through RS validation;
  - with the `oauth2` provider, external tokens are opaque, so only a **non-JWT** Bearer token goes through RS validation.
  In both cases self-signed session JWTs still take the original path, so **there is no regression**.
- **What is validated (`oidc` provider; failing any check means 401)**:
  1. the **signature**, using the IdP's public key (JWKS);
  2. the **issuer**, which must equal the issuer of the IdP in the OIDC configuration;
  3. the **audience**: the token's `aud` must contain the configured `Audience` (this binds the token to this service and prevents replay of tokens the IdP issued for other applications);
  4. the **expiry** `exp`.
  (What the `oauth2` provider validates, and whether it checks `aud` at all, depends on `RSVerifyMethod` — see 2.4.)
- **User mapping**: the username claim is taken from the IdP response (reusing `Attributes.Username` from the corresponding login configuration; `sub` by default, and it can be changed to `preferred_username`) and mapped to a local user.
- **Just-in-time user creation (JIT)**: when no such user exists, one is created following the same rules as the corresponding login configuration. For `oidc`: `Belong=oidc`, roles from the OIDC `DefaultRoles`, teams from the OIDC `DefaultTeams`. For `oauth2`: `Belong=oauth2`, roles from the OAuth2 `DefaultRoles` (the OAuth2 configuration has no default teams). Existing users are never created twice.
- **Permissions**: the user's own roles inside n9e apply. This release does not add any extra authorization based on OAuth scopes.

## 2. Configuration (in two places)

Connecting to an IdP requires two changes: (1) the `[HTTP.RSAuth]` switch in the n9e configuration file, and (2) the OIDC login configuration (which names the trusted IdP).

### 2.1 Configuration file `etc/config.toml`: `[HTTP.RSAuth]`

```toml
[HTTP.RSAuth]
# Master switch. When true, endpoints that go through tokenAuth (a2a/mcp and others) start
# accepting OAuth access tokens from the external IdP
Enable = true
# Resource identifier of this service; the access token's aud must contain it. Required when
# Enable=true — if left empty, RS validation has no effect
Audience = "n9e-a2a-rs"
# Protocol of the trusted IdP: oidc (default, JWT validated locally via JWKS) or oauth2
# (opaque token; see 2.4 for the validation method)
Provider = "oidc"
```

| Field | Type | Default | Description |
|---|---|---|---|
| Enable | bool | false | Master switch for RS validation. When off, the whole branch is skipped and behavior is exactly as before |
| Audience | string | "" | Resource identifier of this service. **RS does not take effect when empty.** Note: only `oidc` and `oauth2`+`introspect` actually validate `aud`; `oauth2`+`userinfo` (the oauth2 default) **does not validate aud** (see 2.4) |
| Provider | string | "oidc" | Protocol of the trusted IdP. `oidc` = reuse the OIDC login and validate JWTs locally via JWKS; `oauth2` = reuse the OAuth2 login and validate opaque tokens (see 2.4) |

The configuration struct is defined as `RSAuth` in `pkg/httpx/httpx.go`. Changes to `config.toml` require a **restart** of center to take effect.

### 2.2 OIDC login configuration (`Provider=oidc`): naming the trusted IdP

RS reuses the IdP from the OIDC configuration, so OIDC must be configured first and **`Enable = true`** (otherwise RS cannot obtain the provider/JWKS and has no effect).
The OIDC configuration lives in the `sso_config` database table and is maintained through the **web UI (System settings → Single sign-on → OIDC)** or the `PUT /api/n9e/sso-config` endpoint — **not** in config.toml.

The fields that matter to RS:

```toml
Enable = true
# The IdP's issuer root URL; n9e uses it to fetch <SsoAddr>/.well-known/openid-configuration and the JWKS
SsoAddr = 'https://idp.example.com/realms/yourrealm'
ClientId = '<oidc-client-id>'
ClientSecret = '<oidc-client-secret>'
DefaultRoles = ['Standard']      # default roles assigned to JIT-created users
DefaultTeams = [2]               # default team IDs a JIT-created user joins (may be empty)

[Attributes]
# Which claim of the access token RS uses as the username; Keycloak commonly uses preferred_username
Username = 'preferred_username'
Nickname = 'name'
Email = 'email'
```

> Note: RS validates the audience against `[HTTP.RSAuth].Audience`, **not** the OIDC `ClientId` — the two are usually different.
> The OIDC `ClientId`/`Secret` are only used for the interactive login flow; RS only needs `SsoAddr` (for the issuer/JWKS), `Attributes.Username`, `DefaultRoles`, and `DefaultTeams`.

### 2.3 On the IdP side: make the access token carry `aud`

Most IdPs will not put your resource identifier into the access token's `aud` by default, so you have to configure an audience explicitly:

- **Keycloak**: add an *Audience* protocol mapper to the client (Client scopes → your scope → Mappers → Add → Audience),
  set `Included Custom Audience` to `n9e-a2a-rs`, and tick *Add to access token*.
- **Auth0**: pass `audience=n9e-a2a-rs` when requesting the token (register that Identifier under APIs).
- **Entra ID**: configure the Application ID URI / scope under *Expose an API* so that the access token's `aud` is that value, and set `Audience` to match.

Also make sure that:

- (with `Provider=oidc`) the IdP issues **JWT access tokens**; if your IdP only issues opaque tokens, switch to `Provider=oauth2` (see 2.4).
- the n9e process can reach the IdP's discovery and JWKS URLs. **If your environment uses an HTTP proxy**, add the IdP address to `NO_PROXY`/`no_proxy`, otherwise fetching the JWKS/discovery document will fail.

### 2.4 OAuth2 login configuration (`Provider=oauth2`): validating opaque tokens

Use this provider when the IdP only issues opaque (non-JWT) access tokens. RS reuses the **OAuth2 login** configuration (System settings → Single sign-on → OAuth2, stored in the `sso_config` table), so OAuth2 must be configured first and **`Enable = true`**. The fields that matter to RS:

```toml
Enable = true
SsoAddr = 'https://sso.example.com/oauth2/authorize'   # advertised as authorization_servers per RFC 9728
UserInfoAddr = 'https://api.example.com/api/v1/user/info'
ClientId = '<client-id>'
ClientSecret = '<client-secret>'                       # used for Basic Auth against the introspection endpoint
DefaultRoles = ['Standard']                            # default roles for JIT-created users (OAuth2 has no default teams)
# Validation method: empty (default) / userinfo, or introspect
RSVerifyMethod = ''
IntrospectAddr = ''                                    # required when RSVerifyMethod=introspect (RFC 7662 introspection endpoint)
IntrospectCacheSeconds = 60                            # seconds to cache positive results keyed by token hash (introspect further caps this at the token's exp); 0 disables caching

[Attributes]
Username = 'sub'
```

The two `RSVerifyMethod` options:

| Value | Validation method | Validates aud | Suitable for |
|---|---|---|---|
| `''` (default) / `userinfo` | Calls `UserInfoAddr` with the token; **success means the token is considered valid** | **No** — the UserInfo response contains no `aud`, so any valid token from the same IdP is accepted | The easiest to integrate (most OAuth2 servers offer UserInfo); use it when security requirements are modest |
| `introspect` | RFC 7662 introspection (`IntrospectAddr`, with ClientId/Secret Basic Auth), validating `active` and `aud` | **Yes** — `aud` must contain `Audience`, otherwise 401 | Use it when you have security requirements |

> ⚠️ **Security note**: `userinfo` is the **default** mode for oauth2 and it **does not validate the audience** — even if you set `[HTTP.RSAuth].Audience`, in this mode that value is only used for RFC 9728 metadata advertisement and **plays no part in the accept/reject decision**. If the same IdP also issues tokens to other applications, n9e will accept those tokens too. **When you need audience binding, switch `RSVerifyMethod` to `introspect`.** The startup log prints a warning for the userinfo mode.

## 3. Integration steps (Keycloak example)

1. **Keycloak**: create or pick a realm and a client; add an Audience mapper so that the access token's `aud` contains `n9e-a2a-rs`; confirm the username lands in `preferred_username`.
2. **n9e config.toml**: set `[HTTP.RSAuth] Enable = true` and `Audience = "n9e-a2a-rs"`, then restart center.
3. **n9e OIDC**: in the web UI under System settings → Single sign-on → OIDC, set `Enable=true`, point `SsoAddr` at the Keycloak realm, fill in `ClientId`/`Secret`, set `Attributes.Username = preferred_username`, and set `DefaultRoles` / `DefaultTeams` as needed. Save (it hot-reloads within about 9 s).
4. **Self-test with a token**: obtain a user's access token from Keycloak and call a2a/mcp (see below).
5. **Verify**: the call returns something other than 401 (MCP returns the tool list, A2A enters the protocol handler); the n9e log shows `[A2A] done ... user=<that user>` / `[MCP] done ... user=<that user>`; and if the user did not exist before, they now appear on the user management page with the default roles/teams.

## 4. Self-test with curl

```bash
# 1) Get the user's access token from the IdP (Keycloak password grant example)
TOKEN=$(curl -s --noproxy '*' \
  -d 'grant_type=password' -d 'client_id=<client>' -d 'client_secret=<secret>' \
  -d 'username=carol' -d 'password=<pwd>' -d 'scope=openid' \
  'https://idp.example.com/realms/yourrealm/protocol/openid-connect/token' | jq -r .access_token)

# 2) Call MCP (a valid token → 200 with result.tools)
curl -s --noproxy '*' -X POST http://127.0.0.1:17000/mcp \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'

# 3) Counter-examples: wrong aud / expired / corrupted signature / wrong iss → always 401 unauthorized
```

## 5. Behavior and boundaries

- **Turning it on and off**: with `Enable=false` the RS branch is skipped entirely, OAuth tokens are no longer accepted, and all other authentication behaves exactly as before.
- **Coexistence, not exclusion**: once enabled, `X-User-Token` and self-signed JWTs behave unchanged; satisfying any one of the three is enough.
- **Scope (agent surface only)**: OAuth access tokens are **only accepted on the `/a2a` and `/mcp` endpoints**. Those two groups install the `agentOAuthScope` marker ahead of `tokenAuth`, and `tokenAuth` only takes the RS / built-in-AS branch when the marker is present. Other endpoints that go through `tokenAuth` (the `/api/n9e/*` management APIs) never see the marker, so such a token falls through to session-JWT validation and ends in a 401. This way a token issued for an agent cannot be used against other endpoints (least privilege). `X-User-Token` (PAT) and browser session JWTs are unaffected and behave unchanged.
- **Discovery chain (see section 6)**: when RS is enabled, n9e publishes `/.well-known/oauth-protected-resource`, adds an oidc entry to the AgentCard, and includes a `WWW-Authenticate: Bearer resource_metadata="…"` header in 401 responses from `/a2a` and `/mcp`. With all three discovery entry points in place, MCP clients can discover the trusted IdP automatically with zero configuration.

## 6. Discovery chain (automatic OAuth discovery)

To reduce manual configuration on the caller's side, when RS is enabled (`rsAuthEnabled`) n9e actively exposes three pieces of "discovery" information so that OAuth-capable clients can find the trusted IdP automatically:

- **An `oidc` entry in the AgentCard** (used by A2A clients, **only when `Provider=oidc`**): in `GET /.well-known/agent-card.json`, `securitySchemes` gains an `oidc` entry (`type=openIdConnect`, with `openIdConnectUrl` pointing at the IdP's `…/.well-known/openid-configuration`) alongside the existing `x-user-token`, and it is added to the `security` array — **either one is sufficient**, and A2A clients use this to choose the OAuth path automatically. With `Provider=oauth2`, a pure OAuth2 IdP has no OIDC discovery document, so the AgentCard does **not** gain that entry (only `x-user-token` remains).
- **RFC 9728 resource metadata endpoint** (used by OAuth/MCP clients): `GET /.well-known/oauth-protected-resource` (public, no authentication) returns:

  ```json
  {
    "resource": "n9e-a2a-rs",
    "authorization_servers": ["https://idp.example.com/realms/yourrealm"],
    "bearer_methods_supported": ["header"]
  }
  ```

  `resource` is `[HTTP.RSAuth].Audience` (an https URL is recommended so it matches RFC 9728 exactly), and `authorization_servers` is the `SsoAddr` of the trusted provider (from OIDC for `oidc`, from OAuth2 for `oauth2`). When RS is not enabled the endpoint returns 404 and advertises nothing. The endpoint is also registered under the path-suffixed aliases `/.well-known/oauth-protected-resource/a2a` and `/.well-known/oauth-protected-resource/mcp` (the RFC 9728 well-known-URI insertion path, which some MCP clients derive from the endpoint they connect to); the content is the same as at the root path.

- **The 401 `WWW-Authenticate` discovery header**: when RS is enabled, **401** responses from the `/a2a` and `/mcp` endpoints carry `WWW-Authenticate: Bearer resource_metadata="<base>/.well-known/oauth-protected-resource"` (with `error="invalid_token"` appended when a Bearer token was present but failed validation). This is exactly the standard automatic-discovery entry point for **MCP clients** (ChatGPT / Claude connectors): call without a token → receive a 401 plus the pointer → fetch the resource metadata → find the trusted IdP → go through OAuth, **with no manual IdP or audience configuration**. `base` is taken from `[HTTP.A2A].BaseURL` if set, otherwise derived from the request's `Host` plus `X-Forwarded-Proto` (the same source as the AgentCard).
  The header is **only attached to `/a2a` and `/mcp`** (implemented by the `rsAuthChallenge` middleware); 401s from the other APIs that share `tokenAuth` (such as the browser session-JWT login flow) do **not** carry it and are unchanged. It is likewise absent when RS is not enabled.

> Note: like the resource metadata endpoint, the AgentCard's `oidc` entry is **computed live on every request** — after enabling RS/OIDC or switching IdP at runtime, the next AgentCard fetch reflects it, with **no restart of center required**.

## 7. Troubleshooting

| Symptom | Possible cause |
|---|---|
| Every OAuth token gets a 401 | `RSAuth.Enable=false` / `Audience` is empty / the corresponding provider is not enabled (OIDC or OAuth2 `Enable=false`) / `oauth2`+`introspect` is missing `IntrospectAddr` / `oauth2`+`userinfo` is missing `UserInfoAddr`. The startup log prints a matching warning |
| A valid token still gets a 401 | (oidc) `aud` does not contain `Audience`, `iss` does not match the `SsoAddr` issuer, the token is expired, or the JWKS cannot be fetched; (oauth2 introspect) `active=false`, `aud` does not contain `Audience`, the introspection endpoint is unreachable, or Basic Auth failed; (oauth2 userinfo) UserInfo returned a non-200 |
| A user is created but with the wrong username | `Attributes.Username` in the corresponding login configuration points at the wrong claim (for example `sub` was configured where `preferred_username` was needed) |
| No user is created automatically, or the user joins no team | `DefaultRoles` / `DefaultTeams` are not configured (OAuth2 has no default teams) |

Enabling debug logs shows the reason for a validation failure: `[RS] verify access token failed: <err>`.

## 8. Code involved

- `pkg/httpx/httpx.go` — the `RSAuth` configuration (`Enable` / `Audience` / `Provider`)
- `pkg/oidcx/oidc.go` — `VerifyAccessToken` (`oidc` provider: reuses the provider's JWKS for signature validation plus issuer/audience/expiry checks, and maps the claim)
- `pkg/oauth2x/oauth2x.go` — `VerifyAccessToken` (`oauth2` provider: introspect/userinfo validation plus caching keyed by token hash)
- `center/router/router_rsauth.go` — `rsAuthProvider` / `rsAuthEnabled` / `shouldVerifyAsRS` (distinguishes tokens per provider) / `authByIdPAccessToken` (JIT user creation) / `oidcDiscoveryURL` / `rsAuthServerAddr` / `oauthProtectedResource` (RFC 9728 metadata) / `rsAuthChallenge` (the 401 `WWW-Authenticate` discovery-header middleware) / `wwwAuthenticateChallenge` / `protectedResourceMetadataURL`
- `center/router/router_mw.go` — the RS branch in `tokenAuth()`; `agentOAuthScope` (restricts OAuth acceptance to the agent surface, gating the branch up front)
- `center/router/router_a2a.go` — registers `/.well-known/oauth-protected-resource` (including the `/a2a` and `/mcp` path aliases), inserts `rsAuthChallenge` into the a2a/mcp middleware chain, and passes the OIDC discovery URL into the AgentCard
- `aiagent/a2a/agent_card.go` — the `oidc` securityScheme entry in the AgentCard
