# Business groups

Business groups (业务组, "BG") are n9e's RBAC / ownership unit: every alert rule, target,
dashboard, mute, subscribe, recording rule, etc. belongs to exactly one business group, and a
user's access to those resources is granted per-group (via teams / user-groups). **The `id` of a
business group here is exactly what other endpoints refer to as `bgid`, `gid`, or `gids`** (e.g.
`/busi-group/<id>/alert-rules`, or the `gids` / `bgid` query on `/busi-groups/alert-rules`,
`/targets`, `/alert-his-events/list`, …). Start here to discover which group ids you may pass to
those endpoints.

> Gateway call: GET, `path` relative to `/api/n9e`. Response `{"ok":true,"status":200,"data":{"dat":<payload>,"err":""}}` — read `data["dat"]`. Protocol: see `../n9e-api.md`.

## Endpoints
| Path | Purpose | `dat` shape |
|---|---|---|
| `/busi-groups` | Business groups you can see (admin: all; others: groups your teams own). | Pattern B — bare array of `BusiGroup` |
| `/busi-group/:id` | One group, **with its owning teams** (`user_groups` filled). `:id` in path. | Single `BusiGroup` object |
| `/busi-groups/tags` | Distinct target tags used across your groups. | Bare array of strings |

Notes:
- `/busi-groups` fills `update_by_nickname` but **not** `user_groups` (that stays empty/null). Use
  `/busi-group/:id` when you need the owning teams and their permission flags.
- `/busi-groups/tags` returns tag strings (e.g. `"env=prod"`) collected from the targets of the
  selected groups — it accepts an optional `gids` query (comma-separated group ids; empty = all
  your groups), and returns `[]string`.

## Query parameters (`/busi-groups`)
| Param | Type | Required | Default | Meaning |
|---|---|---|---|---|
| `query` | string | no | `""` | Case-insensitive substring match on group `name`. (Hidden fallback: for admins, if no name matches, it is retried as a target `ident` to find that host's groups.) |
| `limit` | int (string) | no | `300` | Max rows returned. |
| `all` | bool (string) | no | `false` | `true` = list every group in the system (admins always see all regardless of this flag); otherwise limited to groups your teams own. |

(All query values are strings over the gateway, e.g. `{"limit":"300","all":"true"}`.)

## Response — `dat` payload
(`/busi-groups` = Pattern B bare array.) Each item is a `BusiGroup`:

| Field (`json`) | Type | Meaning |
|---|---|---|
| `id` | int64 | Group id — the value used as `bgid` / `gid` / `gids` elsewhere. |
| `name` | string | Group display name (unique). |
| `label_enable` | int | `1` = this group also injects a label onto its targets' metrics; `0` = off. |
| `label_value` | string | The label value injected when `label_enable=1` (empty otherwise). |
| `create_at` | int64 | Creation time, unix seconds. |
| `create_by` | string | Creator username. |
| `update_at` | int64 | Last-updated time, unix seconds. |
| `update_by` | string | Last-updater username. |
| `update_by_nickname` | string (computed) | Display nickname resolved from `update_by`. |
| `user_groups` | array (computed) | Owning teams + permission flag. **Only populated by `/busi-group/:id`**; empty/null in the `/busi-groups` list. Each element: `{"user_group": <UserGroup>, "perm_flag": "ro"\|"rw"}` where `perm_flag` is `rw` (read-write) or `ro` (read-only), and `UserGroup` carries `id`, `name`, `note`, `create_at`, `create_by`, `update_at`, `update_by`, `update_by_nickname`, and (usually empty here) `users` / `busi_groups`. |

## Example
Request:
```json
{"method":"GET","path":"/busi-groups","query":{"limit":"300"}}
```
Response (trimmed):
```json
{
  "ok": true,
  "status": 200,
  "data": {
    "dat": [
      {
        "id": 2,
        "name": "default-busi-group",
        "label_enable": 0,
        "label_value": "",
        "create_at": 1700000000,
        "create_by": "root",
        "update_at": 1700000000,
        "update_by": "root",
        "update_by_nickname": "Administrator",
        "user_groups": null
      }
    ],
    "err": ""
  }
}
```

One group with owning teams:
```json
{"method":"GET","path":"/busi-group/2","query":{}}
```
```json
{
  "ok": true, "status": 200,
  "data": {
    "dat": {
      "id": 2, "name": "default-busi-group",
      "label_enable": 0, "label_value": "",
      "create_at": 1700000000, "create_by": "root",
      "update_at": 1700000000, "update_by": "root",
      "update_by_nickname": "Administrator",
      "user_groups": [
        {"user_group": {"id": 1, "name": "admins", "note": ""}, "perm_flag": "rw"}
      ]
    },
    "err": ""
  }
}
```
