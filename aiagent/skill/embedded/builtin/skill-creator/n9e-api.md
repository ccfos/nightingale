# n9e read-only API reference (for scripts that call the Skill Gateway)

Read this **before writing any script that queries n9e's own data** (alerts, rules,
targets, dashboards…). Every path/param/shape below is verified against n9e source.

## Golden rule: use ONLY the endpoints listed here — do not invent paths

An unknown path does **not** return 404. n9e serves its single-page-app `index.html`
for any unmatched route, and the gateway hands that **HTML string back in `data`**. So a
wrong path fails *silently* — the script gets a chunk of HTML (often starting with
`<!-- ... Copyright ... Nightingale Team`) instead of JSON. If you're unsure an endpoint
exists, do **not** guess by analogy (there is no `/alert-events`, no `/alert-rules`, no
`/dashboards`) — pick the real one from the catalog below, or tell the user you need them
to confirm the exact `/api/n9e/...` request from their browser dev tools.

## How to call (recap)

- Socket path is in env var `N9E_SKILL_GATEWAY`; talk newline-delimited JSON over it.
- Request: `{"method":"GET","path":"/alert-his-events/list","query":{"hours":"24","limit":"100"}}\n`
  - `path` is relative to `/api/n9e` (with or without the prefix is fine).
  - **GET only** — writes/deletes are rejected.
  - **All `query` values must be STRINGS** — stringify numbers: `{"limit":"100","p":"1"}`, not `100`.
- Response: `{"ok":true,"status":200,"data":<n9e body>}` or `{"ok":false,"status":<code>,"error":"..."}`.

## Response envelope + the two list shapes

`data` is n9e's standard envelope: `{"dat": <payload>, "err": "", "request_id": "..."}` — always
read `data["dat"]`. On an API error `err` is non-empty and there is no `dat` (HTTP is still 200).

**Validate every response** (this is how you catch a wrong path):

```python
if not resp.get("ok") or not isinstance(resp.get("data"), dict):
    # data is a string (HTML) or ok=false → wrong path / denied / error
    raise RuntimeError(f"gateway call failed: {str(resp)[:200]}")
env = resp["data"]
if env.get("err"):
    raise RuntimeError(f"n9e api error: {env['err']}")
dat = env["dat"]
```

`dat` has **two shapes depending on the endpoint** (noted per-endpoint below):

- **Pattern A — paged, has total**: `dat = {"list": [...], "total": N}`. Used by the
  high-volume event/target endpoints. Paginate with `p` (page, from 1) + `limit`.
  Read the rows from `dat["list"]`, the count from `dat["total"]`.
- **Pattern B — bare array, no total**: `dat = [ ...items... ]`. Used by config-object
  lists (rules, boards, mutes, subscribes, busi-groups, teams). Everything is returned
  in one call; there is **no** `total` and no server-side paging — read `dat` directly as a list.

## Business-group scoping idiom

Most business resources come in two variants:

- **Across your groups**: `/busi-groups/<resource>` with optional `gids` query
  (comma-separated group ids; **empty = all groups your RBAC allows**).
- **One group**: `/busi-group/<id>/<resource>` — the `<id>` is **required in the path**.

Events, targets, busi-groups and user-groups are flat (no group id in the path; filter with `bgid`/`gids` query where supported).

## Endpoint catalog (verified)

### Alert events — Pattern A (`{list,total}`)

| Path | Purpose | Key query params |
|---|---|---|
| `/alert-his-events/list` | **Historical** events (fired + recovered). Use for "last 24h" stats. | `hours` (last N h) **or** `stime`/`etime` (unix secs); `p`, `limit` (def 20); `bgid` (optional, 0/absent = all your groups); `severity` (1/2/3); `is_recovered` (0/1); `query` (text); `rid` (rule id); `datasource_ids` (csv); `cate`, `prods` |
| `/alert-cur-events/list` | **Active** (currently-firing) events. | same as above minus `is_recovered`; plus `my_groups` (bool), `event_ids` (csv) |

Time: pass `hours=24` for the last day, or an explicit `stime`/`etime` window — always pass one.

### Alert rules — Pattern B (bare array)

| Path | Purpose | Params |
|---|---|---|
| `/busi-groups/alert-rules` | Rules across groups. | `gids` (csv, optional; empty = your groups / all for admin) |
| `/busi-group/<id>/alert-rules` | Rules of one group. | `<id>` required in path |
| `/alert-rule/<arid>` | One rule's full definition. | `<arid>` in path |

### Targets / hosts

| Path | Purpose | Shape / params |
|---|---|---|
| `/targets` | Monitored targets. | **Pattern A** (`{list,total}`); `gids` (csv, optional), `query`, `limit` (def 30), `p`, `hosts` (csv), `order`, `desc`, `datasource_ids` |
| `/targets/stats` | Target counts/summary. | — |

### Dashboards (boards) — Pattern B

| Path | Purpose | Params |
|---|---|---|
| `/busi-groups/boards` | Boards across groups. | `gids` (csv, optional), `query` |
| `/busi-group/<id>/boards` | Boards of one group. | `<id>` in path; `query` |
| `/boards` | Boards by id. | `bids` (csv of board ids) |
| `/board/<bid>` | One board (with config). | `<bid>` in path |

### Mutes / subscribes / recording rules — Pattern B

| Path | Params |
|---|---|
| `/busi-groups/alert-mutes` | `gids` (csv, optional) |
| `/busi-group/<id>/alert-mutes` | `<id>` in path; `prods`, `query`, `expired` (int, -1=all) |
| `/busi-groups/alert-subscribes` | `gids` (csv, optional) |
| `/busi-group/<id>/alert-subscribes` | `<id>` in path |
| `/busi-groups/recording-rules` | `gids` (csv, optional) |

### Org / metadata — Pattern B

| Path | Purpose | Params |
|---|---|---|
| `/busi-groups` | Business groups you can see. | `query`, `limit` (def 300), `all` (bool) |
| `/user-groups` | Teams. | `query`, `limit` (def 1500) |
| `/metric-views` | Saved metric views. | — |
| `/builtin-metrics` | Built-in metric catalog. | — |
| `/notify-rules` | Notification rules (perm-gated). | — |
| `/event-pipelines` | Event pipelines (perm-gated). | — |

(There are more GET endpoints under `/api/n9e`; the above are the ones skills commonly need.
If you need one that isn't here, confirm the exact path with the user rather than guessing.)

## Reference values

- **severity**: `1` = Emergency/Critical (一级), `2` = Warning (二级), `3` = Notice/Info (三级).
- **is_recovered**: `0` = firing, `1` = recovered.
- **time**: `hours=<N>` (relative, last N hours) or `stime`/`etime` as **unix seconds**.

## Blocked (deny-list) — these return `ok:false`, never write scripts around them

Datasource configs `/datasource*`, notify secrets `/notify-channel*` `/notify-config`,
config-center `/config`, users/tokens `/users` `/user/*` `/self/token` `/user-token`,
SSO/IdP `/sso` `/ldap` `/oidc` `/oauth` `/cas`, datasource proxy `/proxy/*`, `/webhook*`,
`/password*`, and all non-GET methods. These carry secrets or mutate state.
