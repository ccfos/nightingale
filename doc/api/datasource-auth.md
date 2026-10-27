# Datasource Cloud Auth (AWS SigV4 / OAuth2 / Azure Entra ID)

Datasources can authenticate with cloud-managed services in addition to Basic Auth and custom headers:

| Auth type | Typical targets | Datasources |
|------|------|------|
| `sigv4` | Amazon Managed Service for Prometheus (`aps`), Amazon OpenSearch Service (`es`), OpenSearch Serverless (`aoss`) | Prometheus, Loki, Elasticsearch, OpenSearch |
| `oauth2` | Prometheus / Mimir / Loki behind an OAuth2 gateway (Keycloak, Okta, ...) | Prometheus, Loki, Elasticsearch, OpenSearch, VictoriaLogs |
| `azure` | Azure Monitor managed Prometheus | Prometheus, Loki, VictoriaLogs |

The auth is applied at the HTTP transport, so it covers connection tests, dashboards, the datasource proxy (`/api/n9e/proxy/:id/*`), and alert / recording rule evaluation in n9e, n9e-alert and n9e-edge.

## Where credentials are resolved

"Ambient" credentials (AWS default credential chain, Azure Managed Identity / Workload Identity) are resolved on the host of **the process that runs the query**:

| Action | Process |
|------|------|
| Connection test, dashboards, explorer, datasource proxy | n9e (center) |
| Alert rules, recording rules | the alert engine evaluating the rule: n9e, n9e-alert or n9e-edge |

Every one of these processes needs the identity. When the environments differ, use static keys plus AssumeRole so all processes assume the same role.

## Request format

The cloud auth lives in the `auth` object of `POST /api/n9e/datasource/upsert`, next to the existing Basic Auth fields. `type` selects the scheme; only the sub-object matching `type` is persisted. When `type` is `sigv4`, `oauth2` or `azure`, the Basic Auth fields are dropped, and a custom `Authorization` header is rejected.

An empty or missing `type` keeps the legacy behaviour (Basic Auth when `basic_auth_user` is set). Existing datasources need no migration.

### AWS SigV4

```json
{
  "name": "amp",
  "plugin_type": "prometheus",
  "http": { "url": "https://aps-workspaces.us-east-1.amazonaws.com/workspaces/ws-xxxx" },
  "auth": {
    "type": "sigv4",
    "sigv4": {
      "region": "us-east-1",
      "service_name": "aps",
      "credential_type": "default",
      "role_arn": "arn:aws:iam::123456789012:role/n9e-query",
      "external_id": "optional"
    }
  }
}
```

| Field | Description |
|------|------|
| region | Required. |
| service_name | `aps` (default), `es` or `aoss`. Limited by `DatasourceAuth.SigV4AllowedServices`. |
| credential_type | `default`: SDK default chain (env vars, IRSA, EKS Pod Identity, ECS task role, EC2 instance profile). `keys`: `access_key` + `secret_key`. |
| role_arn / external_id | Optional AssumeRole on top of the credential source (cross-account). |
| sts_endpoint | Optional STS endpoint override (VPC endpoint, FIPS). |

Minimum permission for AMP: `aps:QueryMetrics`, `aps:GetLabels`, `aps:GetSeries`, `aps:GetMetricMetadata` (managed policy `AmazonPrometheusQueryAccess`).

### OAuth2 client credentials

```json
"auth": {
  "type": "oauth2",
  "oauth2": {
    "token_url": "https://idp.example.com/oauth2/token",
    "client_id": "n9e",
    "client_secret": "******",
    "scopes": ["metrics.read"],
    "endpoint_params": { "audience": "mimir" }
  }
}
```

Tokens are cached and refreshed before expiry. The token endpoint is called with the datasource's TLS settings.

### Azure Entra ID

```json
"auth": {
  "type": "azure",
  "azure": {
    "cloud": "AzurePublic",
    "credential_type": "client_secret",
    "tenant_id": "...",
    "client_id": "...",
    "client_secret": "******",
    "scope": ""
  }
}
```

| Field | Description |
|------|------|
| cloud | `AzurePublic` (default), `AzureChina`, `AzureGovernment`. |
| credential_type | `client_secret`, `managed_identity` (`client_id` = user-assigned identity, empty = system-assigned), `workload_identity` (tenant/client id default to `AZURE_TENANT_ID` / `AZURE_CLIENT_ID` injected by AKS). |
| scope | Empty = default of the cloud: `https://prometheus.monitor.azure.com/.default` (Public). The China (`.azure.cn`) and US Government (`.azure.us`) defaults follow the same pattern but are not yet verified against Microsoft docs; set `scope` explicitly if token requests fail. |

Grant the identity `Monitoring Data Reader` on the Azure Monitor workspace.

## Errors

A failed connection test returns `{"error": "[<code>] <message>"}`. The code prefix is stable for API callers:

| Code | Meaning |
|------|------|
| `no_credentials` | The process has no usable ambient identity. |
| `assume_role_failed` | AssumeRole was rejected (trust policy, external ID). |
| `token_failed` | OAuth2 / Azure token could not be obtained. |
| `access_denied` | The upstream answered 401/403: the identity lacks permission. |

Use `force_save: true` to save without a connection test, e.g. when only the alert engines have the identity.

## Auth policy

```
GET /api/n9e/datasource/auth-policy
```

Admin only. Returns the switches of the center process so the UI can grey out disabled options:

```json
{
  "data": {
    "allow_ambient_credentials": true,
    "allow_assume_role": true,
    "sigv4_allowed_services": ["aps", "es", "aoss"],
    "azure_default_scopes": { "AzurePublic": "https://prometheus.monitor.azure.com/.default", "...": "..." }
  },
  "error": ""
}
```

Configured per process in `config.toml` / `edge.toml`:

```toml
[DatasourceAuth]
DisableAmbientCredentials = false
DisableAssumeRole = false
SigV4AllowedServices = ["aps", "es", "aoss"]
```

The policy is enforced when saving and again when a client is built, so tightening it also stops existing datasources on that process.

## Secrets

The cloud secrets are stored in the `auth` column like the Basic Auth password: encrypted as a whole into `auth_encoded` when RSA is enabled, never sent in plaintext to edge, and removed from responses for non-admin users.
