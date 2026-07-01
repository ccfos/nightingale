# Teams (user groups)

Teams are named groups of users. They are the unit that notification routing targets and that busi-group membership grants access through — a rule, subscription, or mute can notify "team X", and a business group lists the teams allowed to see it. Use these endpoints to list the teams the caller can see and to read one team together with its member users.

> Gateway call: GET, `path` relative to `/api/n9e`. Response `{"ok":true,"status":200,"data":{"dat":<payload>,"err":""}}` — read `data["dat"]`. Protocol: see `../n9e-api.md`.

## Endpoints
| Path | Purpose | `dat` shape |
|---|---|---|
| `/user-groups` | Teams the caller can see (filtered by `query`) | Pattern B — bare array of `UserGroup` |
| `/user-group/:id` | One team with its member users (`:id` in path) | Single `UserGroup` object |

## Query parameters (`/user-groups`)
| Param | Type | Required | Default | Meaning |
|---|---|---|---|---|
| `query` | string | no | `""` | Case-insensitive text filter over team name / note; empty returns all visible teams |
| `limit` | int | no | `1500` | Max number of teams returned |

## Response — `dat` payload
(`/user-groups` = Pattern B bare array; `/user-group/:id` = single object, includes members.) Each team is a `UserGroup`:
| Field (`json`) | Type | Meaning |
|---|---|---|
| `id` | int64 | Team ID |
| `name` | string | Team name |
| `note` | string | Description / note |
| `create_at` | int64 | Creation time (unix seconds) |
| `create_by` | string | Username of creator |
| `update_at` | int64 | Last update time (unix seconds) |
| `update_by` | string | Username of last updater |
| `update_by_nickname` | string | Nickname of last updater (computed) |
| `users` | array | Member users of the team (computed). Populated on `/user-group/:id`; may be empty/omitted in the list response. Each element has `id` (int64), `username` (string, login), `nickname` (string, display name), plus other user fields. |
| `busi_groups` | array | Business groups this team belongs to (computed); each element carries `id` and `name`. Typically empty unless explicitly loaded. |

## Example
Request:
```json
{"method":"GET","path":"/user-groups","query":{"limit":"1500"}}
```
Response (trimmed):
```json
{
  "ok": true,
  "status": 200,
  "data": {
    "dat": [
      {
        "id": 1,
        "name": "ops-oncall",
        "note": "primary on-call rotation",
        "create_at": 1710000000,
        "create_by": "root",
        "update_at": 1710500000,
        "update_by": "root",
        "update_by_nickname": "Administrator",
        "users": [],
        "busi_groups": []
      }
    ],
    "err": ""
  }
}
```

Reading one team with members — request `{"method":"GET","path":"/user-group/1"}` — returns a single object where `users` is filled:
```json
{
  "id": 1,
  "name": "ops-oncall",
  "users": [
    {"id": 1, "username": "root", "nickname": "Administrator"},
    {"id": 5, "username": "alice", "nickname": "Alice"}
  ],
  "busi_groups": []
}
```
