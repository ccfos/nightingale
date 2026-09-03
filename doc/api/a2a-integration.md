# Opening the A2A Endpoint to Third Parties (Configuration Guide)

The AI assistant built into n9e can be called by external systems as a standard
[A2A (Agent-to-Agent Protocol)](https://a2a-protocol.org/) agent — a third party can drive n9e with a single
sentence of natural language to look up alerts, query data, create rules, or run diagnostics.

This document only covers **what an administrator has to configure** and what information to hand over to the
third party. Protocol details are the responsibility of their A2A client and are not your concern.

## 1. Prerequisites

1. **Configure a large language model**: go to `Integrations → AI configuration` and make sure the AI assistant
   can hold a normal conversation in the UI. A2A uses that same assistant — if it does not work in the UI, it
   will not work over A2A either.
2. **Confirm TokenAuth is enabled** (in `etc/config.toml`; it is on by default):

   ```toml
   [HTTP.TokenAuth]
   Enable = true
   HeaderUserTokenKey = "X-User-Token"
   ```

   If this is turned off, the A2A endpoint rejects every request.

## 2. A2A configuration options

The A2A endpoint is **enabled by default and needs no configuration**. To adjust it, edit `etc/config.toml`:

```toml
[HTTP.A2A]
# Disable = false     # set to true to close every public A2A and MCP endpoint
# DisableMCP = false  # set to true to close only MCP and keep A2A
# BaseURL = ""        # the absolute address advertised publicly, e.g. https://n9e.example.com
```

| Option | Default | Description |
|---|---|---|
| `Disable` | `false` | Closes every public A2A and MCP endpoint |
| `DisableMCP` | `false` | Closes only the MCP endpoints; A2A is unaffected |
| `BaseURL` | empty | The address a third party gets when discovering n9e. When empty it is inferred from the request's `Host` header; **setting it explicitly is recommended when running behind a reverse proxy or load balancer**, otherwise the third party may end up with an internal address |

Restart center for changes to take effect.

## 3. Reverse proxy rules

The A2A endpoints live at the **root path**, not under the `/api/n9e` prefix. If your nginx only forwards
`/api/n9e/*`, you need to allow two more paths:

```nginx
# Discovery address; third parties use it to find n9e
location /.well-known/ {
    proxy_pass http://n9e_center;
}

# A2A endpoint
location /a2a/ {
    proxy_pass http://n9e_center;
    proxy_http_version 1.1;

    proxy_buffering off;        # required: streaming answers depend on it, without it nothing is ever emitted
    proxy_read_timeout 3600s;   # required: a single question may run for several minutes
    proxy_send_timeout 3600s;
}
```

These timeout and buffering settings are the easiest thing to get wrong: the default 60 s timeout cuts off
slightly complex questions in the middle of the answer.

## 4. Creating an API token for the integration

An A2A request runs as the user the token belongs to, so **n9e's business group and role permissions apply
exactly as usual**.

1. Log into n9e, open Profile from the top right → **API Token** → create one, and copy the generated string;
2. We recommend creating **a dedicated account per integration** with least-privilege authorization instead of
   using an administrator's token;
3. Leaking a token is equivalent to leaking the account; you can delete it on the same page at any time to revoke it.

## 5. What to hand over to the third party

Three things are enough:

| Item | Value |
|---|---|
| Discovery address (Agent Card) | `https://n9e.example.com/.well-known/agent-card.json` |
| Authentication | The `X-User-Token: <token>` request header |
| Token | The string generated in the previous step |

Given only the discovery address, a standard A2A client can read out the endpoint, the capability list, and the
authentication method automatically.

## 6. Verifying that it works

```bash
# 1) The discovery address should return JSON with fields such as name and skills (no token needed for this step)
curl -s https://n9e.example.com/.well-known/agent-card.json

# 2) Ask a question with the token; getting an answer back means the integration works
curl -s -X POST https://n9e.example.com/a2a/message:send \
  -H 'Content-Type: application/json' \
  -H 'X-User-Token: <your-token>' \
  -d '{"message":{"messageId":"test-1","role":"ROLE_USER",
       "parts":[{"text":"Which alert events are currently firing?"}]},
       "metadata":{"lang":"en_US"}}'
```

Step 2 may take tens of seconds (the model is thinking and calling tools), which is normal.

| Symptom | What to do |
|---|---|
| It returns `unauthorized` | The token is invalid or has been deleted, or `HTTP.TokenAuth.Enable = false` |
| The discovery address returns 404 | `[HTTP.A2A] Disable = true`, or the reverse proxy does not allow `/.well-known/` |
| No output at all, connection drops after about 60 seconds | nginx is missing `proxy_buffering off` and `proxy_read_timeout` |
| The answer says the model is unavailable | The LLM is not configured properly; verify by chatting with the AI assistant in the UI first |

## 7. Enterprise authentication (optional)

The API token from section 4 is already enough. If you also have either of the following needs, you can switch to
OAuth instead — the integrating party no longer needs a token from you and instead authorizes with their own account:

| Your situation | Which option to use |
|---|---|
| The company already has single sign-on (Keycloak / Okta / Entra ID / Auth0, etc.) | **Option A**: have n9e trust credentials issued by the company's login system |
| No single sign-on, but you want generic clients such as Claude or ChatGPT to connect to n9e directly | **Option B**: have n9e act as the authorization server itself |

Both options can be enabled at the same time without interfering with each other, and API tokens keep working.

### 7.1 Option A: connect to the company's existing login system

The integrating party calls n9e as **the employee themselves**, so permissions and the audit trail belong to that
person. When someone leaves, disabling them in the company's login system is enough; nothing has to change in n9e.

**Step 1: register an identifier for n9e in the company's login system.**
The credentials it issues have to state "this is for n9e" — give it a name such as `n9e-a2a-rs`.
With Keycloak: `Client scopes → the relevant scope → Mappers → Add → Audience`,
set `Included Custom Audience` to `n9e-a2a-rs`, and tick *Add to access token*.
Auth0, Entra ID, and others configure the same thing under their own "API / audience" settings, though the
naming differs from vendor to vendor.

**Step 2: edit the n9e configuration file `etc/config.toml`, then restart center.**

```toml
[HTTP.RSAuth]
Enable = true
Audience = "n9e-a2a-rs"   # must match exactly what you entered in the login system in step 1
Provider = "oidc"         # use this when the company login system speaks OIDC (most cases)
```

**Step 3: configure single sign-on in the n9e UI.**
Go to `System settings → Single sign-on → OIDC`:

- turn the switch on;
- fill in the company login system's address, ClientId, and ClientSecret;
- the username field (Attributes → Username) is usually `preferred_username`;
- set the default roles and default teams — the first time an employee calls in this way, n9e creates an account
  for them automatically using exactly these defaults.

The change takes effect about 10 seconds after saving; no restart is needed for this step.

> Additional notes:
> - The server running n9e must be able to reach the company login system's address; if the environment uses an
>   HTTP proxy, remember to add that address to `no_proxy`.
> - If the company uses plain OAuth2 rather than OIDC, set `Provider` to `"oauth2"` and configure it under
>   `System settings → Single sign-on → OAuth2`. That mode does not validate the identifier from step 1 by default,
>   so higher security requirements call for extra settings — see [a2a-oauth-rs.md](./a2a-oauth-rs.md).

Once this is configured, the only thing you still hand to the third party is the discovery address from section 5 —
their client discovers which login system to use on its own, and you do not have to tell them anything else.

### 7.2 Option B: have n9e act as the authorization server

This suits environments without single sign-on where you want generic clients such as Claude or ChatGPT to
"connect by just entering an address". You do not issue tokens up front; the user completes the authorization with
a single click in their client.

**Step 1: edit `etc/config.toml`, then restart center.**

```toml
[HTTP.MCPAuth]
Enable = true
# The address at which users' browsers actually reach n9e (the same one you enter in step 3 below)
Issuer = "https://n9e.example.com"
```

**Step 2: allow one more path in the reverse proxy** (in addition to the two locations in section 3):

```nginx
location /oauth/ {
    proxy_pass http://n9e_center;
}
```

If you are integrating an MCP client such as Claude or ChatGPT, you also need to allow `/mcp`, configured exactly
like `/a2a/` in section 3 (buffering off, generous timeouts):

```nginx
location /mcp {
    proxy_pass http://n9e_center;
    proxy_http_version 1.1;

    proxy_buffering off;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
}
```

**Step 3: tell users which address to enter in their client.**

Pick one according to the client type, replacing `n9e.example.com` with your own domain:

| Client type | Address to enter |
|---|---|
| MCP clients such as Claude and ChatGPT (the UI usually calls it "Add custom connector" and asks for an MCP server address) | `https://n9e.example.com/mcp` |
| Standard A2A clients | `https://n9e.example.com/.well-known/agent-card.json`; some clients only want the root address `https://n9e.example.com` and append the rest themselves |

This address has to satisfy three conditions, otherwise authorization cannot complete:

1. **It must be reachable from the user's browser** — the authorization flow involves logging into n9e in the
   browser and clicking to confirm, so it cannot be an IP, container name, or `127.0.0.1` that only the server can reach;
2. **The domain, scheme, and port must match the `Issuer` from step 1 exactly** — if `Issuer` is
   `https://n9e.example.com`, you cannot enter `http://` or a form with a port here;
3. **Use HTTPS** — for security reasons most clients do not accept an `http://` remote server address
   (`localhost` for local debugging is the exception).

After entering it, the user goes through: the client discovers automatically that n9e is the authorization server →
the browser opens the n9e login page (skipped if already logged in) → the authorization confirmation page appears
and they click "Allow" → they return to the client and the connection is complete.
From then on that client calls n9e as that person, with the same permissions they have when logged into the UI.

> In a multi-instance deployment (several center instances behind a load balancer), `Issuer` must be **set explicitly
> and identically on every instance** — it cannot be left empty. When empty, each instance infers it from the request
> it received, which may diverge, so an authorization completed on instance A is not recognized by instance B. The
> signing key is managed centrally through the database shared by all instances and needs no extra configuration.

