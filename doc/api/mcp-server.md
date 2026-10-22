# Connect an MCP client to Nightingale

Nightingale's `n9e` process includes a [Model Context Protocol](https://modelcontextprotocol.io/) server at `/mcp`. It uses Streamable HTTP and needs no separate MCP server process.

Use it to query alerts, inspect metrics and logs, and manage alerting from a compatible client such as Claude Code or Cursor. Model execution happens in the client or its model service; the MCP endpoint exposes Nightingale's tools.

## Connect with a personal token

1. Enable personal-token authentication in your Nightingale configuration:

   ```toml
   [HTTP.TokenAuth]
   Enable = true
   ```

   This is enabled in the standalone default configuration, but some deployment examples, including `docker/compose-bridge/etc-nightingale/config.toml`, disable it. Update the existing setting and restart Nightingale if necessary. Leave `HTTP.A2A.Disable` and `HTTP.A2A.DisableMCP` unset or `false`.

2. Sign in to Nightingale and create a personal token under **Profile → Token management**.

3. Configure your MCP client with the server URL and the token in the `X-User-Token` header. For clients that accept the `mcpServers` configuration format:

   ```json
   {
     "mcpServers": {
       "nightingale": {
         "type": "http",
         "url": "http://127.0.0.1:17000/mcp",
         "headers": {
           "X-User-Token": "<your-token>"
         }
       }
     }
   }
   ```

   Use the Nightingale address reachable from the client. The path is `/mcp`, not `/api/n9e/mcp`. Use HTTPS for remote connections and keep the token out of shared configuration files.

4. Ask the client to list the available tools, then try a read request:

   > Which critical alerts are still firing?

## Tools and permissions

Toolsets cover `alerts`, `targets`, `datasource`, `mutes`, `busi_groups`, `notify_rules`, `alert_subscribes`, `event_pipelines`, `users`, `metrics`, `logs`, `dashboards`, and `roles`.

Every tool call is dispatched to Nightingale's existing HTTP API inside the process, carrying the caller's credential. API permissions and business-group checks apply as they do to that user's other API calls.

**Write tools are disabled by default.** To restrict the available toolsets or enable writes, update the existing `[HTTP.A2A]` section:

```toml
[HTTP.A2A]
# An empty list exposes all registered toolsets.
MCPToolsets = ["alerts", "metrics", "dashboards"]

# Set true to expose create, update, and delete tools.
MCPEnableWriteTools = false

# Set true to disable only the MCP endpoint.
DisableMCP = false
```

Restart Nightingale after changing these settings. Use a user with only the permissions your client needs. Enabling write tools permits that client to change resources through the API; it does not add a server-side approval step for each call. Configure any per-action confirmation in the MCP client.

The read-only tool setting controls which tools are registered. Data-source access and database credentials still need to be scoped for the queries you permit.

## Connect with OAuth

The endpoint also accepts OAuth access tokens through `Authorization: Bearer <token>`:

- [Built-in authorization server](mcp-oauth-as.md): let clients discover and register with Nightingale using dynamic client registration and PKCE. Enable and configure this mode before connecting an OAuth client.
- [External identity provider](a2a-oauth-rs.md): use tokens from an OIDC or OAuth2 provider such as Keycloak, Entra ID, or Okta, mapped to a local Nightingale user.

These are separate configuration options; a personal token is sufficient for the example above.

## Related

- [A2A integration](a2a-integration.md) exposes Nightingale's built-in assistant to other agents.
- [Model provider configuration](ai-llm-config.md) configures the model used by that built-in assistant.
- [Standalone MCP server](https://github.com/n9e/n9e-mcp-server) is available if you want to run MCP as a separate process against a remote Nightingale instance.
- [Back to the README](../../README.md#ai-assistant-and-mcp).
