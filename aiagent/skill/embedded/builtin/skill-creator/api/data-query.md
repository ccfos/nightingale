# Data query (metrics & logs)

Query actual time-series / log **data** from a datasource. Unlike every other endpoint
in this reference, these are **POST** (they carry a query body). The gateway allows POST
**only** for the read-only query endpoints below; everything else stays GET-only.

**First get a `datasource_id`** (and its `plugin_type`/`cate`) from `api/datasources.md`
(`GET /datasource/brief`). You cannot guess datasource ids.

> Gateway call for these: **POST**, with the query in `body` (a JSON **object**, not a string).
> Request: `{"method":"POST","path":"/query-range-batch","body":{...}}`. In the body, use
> **native JSON types** (numbers as numbers) — the string-only rule applies to `query`, not `body`.
> Response: `{"ok":true,"status":200,"data":{"dat":<result>,"err":""}}` — read `data["dat"]`.
> Body cap: 256 KiB.

## Endpoints (the POST allowlist)

| Path | Purpose | Body | `dat` result |
|---|---|---|---|
| `/query-range-batch` | Prometheus **range** query (series over a window) | `BatchQueryForm` | array aligned to `queries`; each is a Prometheus **matrix** |
| `/query-instant-batch` | Prometheus **instant** query (one point) | `BatchInstantForm` | array aligned to `queries`; each is a Prometheus **vector** |
| `/ds-query` | **Unified** query for any datasource (metrics or logs) by `cate` | `QueryParam` | datasource-specific |
| `/logs-query` · `/log-query` · `/log-query-batch` | Log query | `QueryParam` | `{"total":N,"list":[...]}` |

## Prometheus metrics — the simplest path

### Range: `/query-range-batch`

Body (`BatchQueryForm`):
```json
{
  "datasource_id": 1,
  "queries": [
    {"query": "<PromQL>", "start": 1751330400, "end": 1751334000, "step": 60}
  ]
}
```
- `datasource_id` (int, required) — from `/datasource/brief`.
- `queries[]` (required), each: `query` (PromQL string), `start`/`end` (unix seconds), `step` (seconds resolution). All required.

`dat` is an array with one entry per query; each entry is a Prometheus **matrix**
(list of series):
```json
{"ok":true,"status":200,"data":{"dat":[
  [ {"metric":{"__name__":"cpu","ident":"host-01"}, "values":[[1751330400,"6.3"],[1751330460,"7.1"]]} ]
],"err":""}}
```

### Instant: `/query-instant-batch`

Body (`BatchInstantForm`) — items use `time`, not start/end/step:
```json
{"datasource_id": 1, "queries": [{"query": "<PromQL>", "time": 1751334000}]}
```
`dat` is an array per query; each entry is a Prometheus **vector**:
`[{"metric":{...}, "value":[1751334000,"7.1"]}]`.

## Any datasource — `/ds-query`

Body (`QueryParam`):
```json
{"cate": "<plugin_type>", "datasource_id": <id>, "query": [ <one-or-more query objects> ]}
```
- `cate` — the datasource's `plugin_type` from `/datasource/brief` (`prometheus`, `mysql`, `elasticsearch`, `loki`, `tdengine`, `ck`, …).
- `query[]` — the query object(s); **shape depends on `cate`**. It matches that datasource's
  alert-rule query config, which is already documented per type — read it with:
  `read_file(base="create-alert-rule", path="datasources/<cate>.md")`
  (e.g. `datasources/prometheus.md`, `datasources/mysql.md`, `datasources/elasticsearch.md`, `datasources/loki.md`).

`dat` is datasource-specific: Prometheus-like sources return metric values; SQL sources
(`mysql`/`pgsql`/`ck`/`tdengine`) return rows; ES/Loki return log/aggregation results.

## Logs — `/logs-query`

Body: `QueryParam` (same shape: `cate` + `datasource_id` + `query[]`; query shape per
`create-alert-rule/datasources/<cate>.md`). `dat` = `{"total": N, "list": [ ...records... ]}`.

## Recommended workflow

1. `GET /datasource/brief` → pick a datasource by `category`/`plugin_type` → note its `id` + `plugin_type`.
2. Prometheus-like metric source → `POST /query-range-batch` (or `/query-instant-batch`) with `datasource_id` + PromQL.
3. Other source, or logs → `POST /ds-query` (or `/logs-query`) with `cate=plugin_type` and the
   query shape from `create-alert-rule/datasources/<cate>.md`.

## Notes

- Read-only and under **your** RBAC — the handler checks your datasource permission (`CheckDsPerm`); no writes.
- Validate the response like any gateway call (`ok` true, `data` is a dict, `data["err"]` empty) — see `../n9e-api.md`.
- If you don't know a datasource's query shape and its `create-alert-rule/datasources/<cate>.md`
  isn't enough, ask the user rather than guessing the body.
