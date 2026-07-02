# Alert mutes (silences)

Alert mutes (a.k.a. silences) suppress alert notifications for events whose tags match the mute's matchers, during a configured time window. The window is either a one-off time range (`mute_time_type=0`, using `btime`/`etime`) or a recurring weekly schedule (`mute_time_type=1`, using `periodic_mutes`). Use these read endpoints to list existing mutes.

> Gateway call: GET. Include the `/api/n9e` prefix in `path`. Response `{"ok":true,"status":200,"data":{"dat":<payload>,"err":""}}` — read `data["dat"]`. Protocol: see `../n9e-api.md`.

## Endpoints
| Path | Purpose | `dat` shape |
|---|---|---|
| `/busi-groups/alert-mutes` | Mutes across all business groups you can read (optionally narrowed by `gids`) | Pattern B: bare array of `AlertMute` |
| `/busi-group/:id/alert-mutes` | Mutes of one business group (`:id` in the path) | Pattern B: bare array of `AlertMute` |

## Query parameters
| Param | Type | Required | Default | Meaning | Endpoint |
|---|---|---|---|---|---|
| `gids` | string (comma-separated ints) | No | all groups you can read | Restrict to these business group IDs. Empty = all groups you have read access to (admins = all groups). | `/busi-groups/alert-mutes` |
| `prods` | string (space-separated) | No | all | Keep only mutes whose `prod` is in this list, e.g. `metric anomaly loki`. | `/busi-group/:id/alert-mutes` |
| `query` | string | No | (none) | Case-insensitive substring filter on the `cause` field. Multiple space-separated words are AND-ed (every word must match). | `/busi-group/:id/alert-mutes` |
| `expired` | int | No | `-1` | Time-window filter. `-1` = all mutes (no filter). `1` = only expired mutes: time-range mutes (`mute_time_type=0`) whose `etime < now`. `0` (or any value other than `-1`/`1`) = only non-expired mutes: time-range mutes with `etime >= now` plus all periodic mutes (`mute_time_type=1`). | `/busi-group/:id/alert-mutes` |

## Response — `dat` payload
(Pattern B bare array.) Each mute is an `AlertMute`:
| Field (`json`) | Type | Meaning |
|---|---|---|
| `id` | int64 | Mute rule ID. |
| `group_id` | int64 | Business group ID this mute belongs to. |
| `note` | string | Free-text note/description. |
| `cate` | string | Category/classification of the mute. |
| `prod` | string | Product / monitoring type the mute applies to (e.g. `metric`, `anomaly`, `loki`). |
| `datasource_ids` | []int64 (computed) | Datasource IDs this mute is scoped to. A single element `[0]` means "all datasources". |
| `cluster` | string | Effective clusters, separated by spaces. |
| `tags` | array of `TagFilter` | Tag matchers — an alert is muted only if its tags satisfy every matcher (see `TagFilter` below). |
| `cause` | string | Reason for creating the mute. |
| `btime` | int64 | Begin time, Unix seconds (used when `mute_time_type=0`). |
| `etime` | int64 | End time, Unix seconds (used when `mute_time_type=0`). |
| `disabled` | int | `0` = enabled, `1` = disabled. |
| `activated` | int (computed) | Whether the mute is in effect right now: `1` = currently within its mute window, `0` = not. |
| `create_by` | string | Username of the creator. |
| `update_by` | string | Username of the last updater. |
| `update_by_nickname` | string (computed) | Display nickname of the last updater. |
| `create_at` | int64 | Creation time, Unix seconds. |
| `update_at` | int64 | Last update time, Unix seconds. |
| `mute_time_type` | int | Window kind: `0` = one-off time range (`btime`/`etime`), `1` = periodic weekly schedule (`periodic_mutes`). |
| `periodic_mutes` | array (computed) | Recurring schedule entries (used when `mute_time_type=1`). Each has `enable_stime`, `enable_etime` (space-separated `HH:MM` lists) and `enable_days_of_week` (e.g. `"0 1 2 3 4 5 6"`). |
| `severities` | []int (computed) | Alert severity levels this mute applies to (e.g. `1`,`2`,`3`). Empty = all severities. |

### `TagFilter` (each item in the mute's `tags` matchers)
| Field (`json`) | Type | Meaning |
|---|---|---|
| `key` | string | Tag key to match against. |
| `func` | string | Match operator: `==`, `!=`, `=~`, `!~`, `in`, or `not in`. (If empty, the server falls back to `op`.) |
| `op` | string | Legacy alias of `func`, same allowed operators; usually empty when `func` is set. |
| `value` | any | Tag value to compare. A string for `==`/`!=`/`=~`/`!~`; for `in`/`not in` it is a space-separated string or an array of values. |

## Example
Request:
```json
{"method":"GET","path":"/api/n9e/busi-groups/alert-mutes","query":{}}
```
Response (trimmed):
```json
{
  "ok": true,
  "status": 200,
  "data": {
    "dat": [
      {
        "id": 12,
        "group_id": 4,
        "note": "silence disk alerts during maintenance",
        "cate": "",
        "prod": "metric",
        "datasource_ids": [0],
        "cluster": "",
        "tags": [
          {"key": "rulename", "func": "==", "op": "", "value": "disk_full"}
        ],
        "cause": "planned maintenance",
        "btime": 1719800000,
        "etime": 1719810000,
        "disabled": 0,
        "activated": 1,
        "create_by": "root",
        "update_by": "root",
        "update_by_nickname": "Administrator",
        "create_at": 1719799000,
        "update_at": 1719799000,
        "mute_time_type": 0,
        "periodic_mutes": [],
        "severities": [1, 2, 3]
      }
    ],
    "err": ""
  }
}
```
