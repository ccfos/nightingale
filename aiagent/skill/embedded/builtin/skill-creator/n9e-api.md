# n9e read-only API reference (for scripts that call the Skill Gateway)

Read this **before writing any script that queries n9e's own data** (alerts, rules,
targets, dashboards…). This file is the index + the rules that apply to every call; the
**per-resource files under `api/`** have the exact params and the full response object
fields. Everything here is verified against n9e source.

## Golden rule: use ONLY documented endpoints — never invent paths

An unknown path does **not** return 404. n9e serves its single-page-app `index.html` for
any unmatched route, and the gateway hands that **HTML string back in `data`**. So a wrong
path fails *silently* — the script gets HTML (often starting with `<!-- ... Copyright ...
Nightingale Team`) instead of JSON. Do **not** guess paths by analogy: there is no
`/alert-events`, no plain `/alert-rules`, no `/dashboards`. Take every path from the
per-resource file below; if what you need isn't documented, ask the user to confirm the
real `/api/n9e/...` request from their browser dev tools rather than guessing.

## How to call

- Socket path is in env var `N9E_SKILL_GATEWAY`; talk newline-delimited JSON over it.
- Request: `{"method":"GET","path":"/alert-his-events/list","query":{"hours":"24","limit":"100"}}\n`
  - `path` is relative to `/api/n9e` (with or without the prefix is fine).
  - **GET only** — writes/deletes are rejected.
  - **All `query` values must be STRINGS**: `{"limit":"100","p":"1"}`, not `100`.
- Response: `{"ok":true,"status":200,"data":<n9e body>}` or `{"ok":false,"status":<code>,"error":"..."}`.

## Response envelope + the two list shapes

`data` is n9e's envelope: `{"dat": <payload>, "err": "", "request_id": "..."}` — always read
`data["dat"]`. On an API error `err` is non-empty and there is no `dat` (HTTP is still 200).

**Validate every response** (this is how you catch a wrong path):

```python
if not resp.get("ok") or not isinstance(resp.get("data"), dict):
    raise RuntimeError(f"gateway call failed (wrong path? denied?): {str(resp)[:200]}")
env = resp["data"]
if env.get("err"):
    raise RuntimeError(f"n9e api error: {env['err']}")
dat = env["dat"]
```

`dat` has **two shapes** (each per-resource file says which):

- **Pattern A — paged, has total**: `dat = {"list": [...], "total": N}`. Paginate with `p`
  (page, from 1) + `limit`. Used by the high-volume event/target endpoints.
- **Pattern B — bare array, no total**: `dat = [ ...items... ]`. Everything returned at once,
  no server-side paging. Used by config-object lists (rules, boards, mutes, subscribes,
  busi-groups, teams).

## Business-group scoping idiom

Most business resources come in two variants:

- **Across your groups**: `/busi-groups/<resource>` with optional `gids` query
  (comma-separated group ids; **empty = all groups your RBAC allows**).
- **One group**: `/busi-group/<id>/<resource>` — the `<id>` is **required in the path**.

Events, targets, busi-groups and user-groups are flat (filter with `bgid`/`gids` query where supported).

## Per-resource references (read the one you need)

`read_file(base="skill-creator", path="api/<file>")`.

| Resource | File | Endpoints (under `/api/n9e`) |
|---|---|---|
| Alert events (history + active) | `api/alert-events.md` | `/alert-his-events/list`, `/alert-cur-events/list`, `/alert-{his,cur}-event/:eid` |
| Alert rules | `api/alert-rules.md` | `/busi-groups/alert-rules`, `/busi-group/:id/alert-rules`, `/alert-rule/:arid` |
| Targets (hosts) | `api/targets.md` | `/targets`, `/targets/stats` |
| Dashboards (boards) | `api/boards.md` | `/boards`, `/busi-groups/boards`, `/busi-group/:id/boards`, `/board/:bid` |
| Alert mutes (silences) | `api/alert-mutes.md` | `/busi-groups/alert-mutes`, `/busi-group/:id/alert-mutes` |
| Alert subscriptions | `api/alert-subscribes.md` | `/busi-groups/alert-subscribes`, `/busi-group/:id/alert-subscribes` |
| Business groups | `api/busi-groups.md` | `/busi-groups`, `/busi-group/:id` |
| Teams (user groups) | `api/user-groups.md` | `/user-groups`, `/user-group/:id` |

Other read endpoints exist under `/api/n9e` (recording rules `/busi-groups/recording-rules`,
metric views `/metric-views`, builtin metrics `/builtin-metrics`, notify rules `/notify-rules`,
event pipelines `/event-pipelines`, …). They're not detailed here — if you need one, confirm
its exact path/params/response with the user before writing to it.

## Reference values (shared)

- **severity**: `1` = Emergency/Critical (一级), `2` = Warning (二级), `3` = Notice/Info (三级).
- **is_recovered**: `0` = firing, `1` = recovered.
- **time** (event endpoints): `hours=<N>` (relative, last N hours) or `stime`/`etime` as **unix seconds**.

## Blocked (deny-list) — return `ok:false`, don't script around them

Datasource configs `/datasource*`, notify secrets `/notify-channel*` `/notify-config`,
config-center `/config`, users/tokens `/users` `/user/*` `/self/token` `/user-token`,
SSO/IdP `/sso` `/ldap` `/oidc` `/oauth` `/cas`, datasource proxy `/proxy/*`, `/webhook*`,
`/password*`, and **all non-GET methods**. These carry secrets or mutate state.
