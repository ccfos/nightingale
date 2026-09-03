# Datasources (discovery)

To query data you need a `datasource_id` (and its `cate`/plugin type). This endpoint
is how a skill discovers them. Datasource **configs** (addresses/credentials) are
deny-listed; only this **secret-redacted** brief is reachable.

> Gateway call: GET. Include the `/api/n9e` prefix in `path`. Response `{"ok":true,"status":200,"data":{"dat":<payload>,"err":""}}` — read `data["dat"]`. Protocol: see `../n9e-api.md`.

## Endpoint

| Path | Purpose | `dat` shape |
|---|---|---|
| `/datasource/brief` | List datasources you can query, with secrets redacted. **The only datasource path reachable via the gateway** (`/datasource*` configs are blocked). | Pattern B (bare array) |

No query params. Returns every datasource (the server redacts secrets).

## Response — `dat` payload

Bare array of datasource objects. Each is a `Datasource` with **secrets stripped**
(`settings`/`auth`/`http` are redacted server-side — no addresses, passwords, tokens):

| Field (`json`) | Type | Meaning |
|---|---|---|
| `id` | int64 | **Datasource id — pass as `datasource_id` to the query endpoints.** |
| `name` | string | Display name. |
| `identifier` | string | Unique identifier. |
| `description` | string | Description. |
| `plugin_id` | int64 | Plugin id. |
| `plugin_type` | string | Plugin type — **this is the `cate` for `/ds-query`** (e.g. `prometheus`, `mysql`, `elasticsearch`, `loki`, `tdengine`, `ck`). |
| `plugin_type_name` | string | Human plugin name (e.g. `Prometheus Like`). |
| `category` | string | Broad category: `timeseries`, `logging`, etc. — use to tell metric vs log sources. |
| `cluster_name` | string | Cluster name (legacy). |
| `status` | string | `enabled` / `disabled`. |
| `is_default` | bool | Whether it's the default datasource. |
| `settings` | object | **Redacted** — only non-sensitive UI keys remain; do not expect addresses/credentials. |
| `created_at` / `updated_at` | int64 | Unix seconds. |
| `created_by` / `updated_by` | string | Author. |

## Example

Request:
```json
{"method":"GET","path":"/api/n9e/datasource/brief"}
```
Response (trimmed):
```json
{"ok":true,"status":200,"data":{"dat":[
  {"id":1,"name":"prod-prometheus","plugin_type":"prometheus","category":"timeseries","status":"enabled","is_default":true},
  {"id":5,"name":"app-loki","plugin_type":"loki","category":"logging","status":"enabled"}
],"err":""}}
```

Then query with the chosen `id` (+ `plugin_type` as `cate`) — see `api/data-query.md`.
