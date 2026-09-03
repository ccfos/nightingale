# Alert events (historical & active)

Fired/recovered alert events. Use these for alert stats, recent-alert summaries, and incident lists. "Historical" = every fired **and** recovered event ever written (server-side paged); "active" = the events currently firing right now.

> Gateway call: GET. **Include the `/api/n9e` prefix in `path`** (e.g. `/api/n9e/alert-his-events/list`). Response `{"ok":true,"status":200,"data":{"dat":<payload>,"err":""}}` — read `data["dat"]`. Protocol + envelope: see `../n9e-api.md`.

## Endpoints
| Path | Purpose | `dat` shape |
|---|---|---|
| `/alert-his-events/list` | Historical events (fired + recovered), server-side paged. Main endpoint for "last N hours" stats. | Pattern A `{"list":[<AlertHisEvent>...],"total":N}` |
| `/alert-cur-events/list` | Currently-active (firing) events, server-side paged. | Pattern A `{"list":[<AlertCurEvent>...],"total":N}` |
| `/alert-his-event/:eid` | One historical event by id (`:eid` in path). | Single object — **`AlertCurEvent` shape** (the server converts the historical row to cur form; note `is_recovered` becomes a bool here) |
| `/alert-cur-event/:eid` | One active event by id (`:eid` in path). | Single object — `AlertCurEvent` |

Notes:
- List rows come pre-enriched: `notify_groups_obj` (resolved user-group objects) is filled on every list row. The `:eid` detail endpoints additionally fill `notify_version` and `notify_rules`.
- Order: newest first (`trigger_time desc`).
- Both `:eid` endpoints return an `AlertCurEvent`-shaped object, so read them with the AlertCurEvent field set below (including the type note on `is_recovered`).

## Query parameters (`/list` endpoints)
| Param | Type | Required | Default | Meaning |
|---|---|---|---|---|
| `hours` | int (string) | one time source | — | Last N hours window. Sets `stime = now - N*3600`, `etime = now + 24h`. |
| `stime` | int (unix secs, string) | one time source | 0 | Window start. Use instead of `hours`. |
| `etime` | int (unix secs, string) | one time source | 0 | Window end (if `stime` set but `etime` omitted, server uses `now + 24h`). |
| `p` | int (string) | no | 1 | Page number, from 1. |
| `limit` | int (string) | no | 20 | Page size. |
| `bgid` | int (string) | no | 0 | Business-group id. 0/absent = all groups your RBAC allows. |
| `severity` | int / csv (string) | no | see note | his: single int, `-1` = all; cur: comma-separated list, empty = all. `1`=Critical, `2`=Warning, `3`=Info. |
| `is_recovered` | int (string) | no | -1 | **his only**: `0`=firing, `1`=recovered, `-1`=all. |
| `query` | string | no | "" | Text substring; matches `rule_name` or `tags` (space-separated terms are AND-ed). |
| `rid` | int (string) | no | 0 | Filter by alert rule id. |
| `datasource_ids` | csv (string) | no | — | Filter by datasource ids. |
| `cate` | string | no | `$all` | Datasource category (e.g. `prometheus`); `$all` = no filter. |
| `prods` | csv (string) | no | "" | Rule products/types (e.g. `metric,host`); empty = all. |
| `my_groups` | bool (string) | no | false | **cur only**: restrict to your own business groups. |
| `event_ids` | csv (string) | no | — | **cur only**: fetch specific event ids. |

Always pass a window (`hours`, or `stime`/`etime`). Without one, historical queries default to `stime=0` (all time).

## Response — `dat` payload

`/list` endpoints: Pattern A — `dat = {"list": [...], "total": N}`; rows are in `dat["list"]`, count in `dat["total"]`. `/:eid` endpoints: a single object.

Each historical list row is an `AlertHisEvent`. Fields marked **(computed)** are `gorm:"-"` — derived server-side, not raw DB columns.

| Field (`json`) | Type | Meaning |
|---|---|---|
| `id` | int64 | Event id (historical id; reused as the active-event id). |
| `cate` | string | Datasource category (e.g. `prometheus`, `host`). |
| `is_recovered` | int | `0` = firing, `1` = recovered. |
| `datasource_id` | int64 | Datasource id the rule ran against. |
| `cluster` | string | Legacy datasource cluster name. |
| `group_id` | int64 | Business group id. |
| `group_name` | string | Business group name. |
| `hash` | string | Dedup hash (rule_id + vector key); identifies the alert series. |
| `rule_id` | int64 | Alert rule id. |
| `rule_name` | string | Alert rule name (rendered). |
| `rule_note` | string | Alert rule note/description. |
| `rule_prod` | string | Rule product/type (e.g. `metric`, `host`, `logging`). |
| `rule_algo` | string | Anomaly-detection algorithm; empty for threshold rules. |
| `severity` | int | `1`=Critical, `2`=Warning, `3`=Info. |
| `prom_for_duration` | int | "for" duration (seconds) the condition must hold before firing. |
| `prom_ql` | string | PromQL (Prometheus/legacy rules). |
| `rule_config` | object | Parsed rule config (structure varies by rule type). (computed) |
| `prom_eval_interval` | int | Rule evaluation interval (seconds). |
| `callbacks` | []string | Callback URLs. (computed) |
| `runbook_url` | string | Runbook URL. |
| `notify_recovered` | int | Whether recovery is notified (`1`/`0`). |
| `notify_channels` | []string | Notify channel keys. (computed) |
| `notify_groups` | []string | Notify user-group ids (as strings). (computed) |
| `notify_groups_obj` | []UserGroup | Resolved user-group objects (filled on list rows). (computed) |
| `target_ident` | string | Target ident (host identifier), if host-scoped. |
| `target_note` | string | Target note. |
| `trigger_time` | int64 | Unix seconds when the event triggered. |
| `trigger_value` | string | The value that triggered the alert. |
| `recover_time` | int64 | Unix seconds when recovered (`0` while firing). |
| `last_eval_time` | int64 | Unix seconds of the last evaluation for this event. |
| `tags` | []string | Event tags as `"k=v"` strings. (computed) |
| `original_tags` | []string | Tags before relabeling. (computed) |
| `annotations` | map[string]string | Annotation key→value map. (computed) |
| `notify_cur_number` | int | Current notification count for this event. |
| `first_trigger_time` | int64 | Unix seconds of the first trigger in a continuous alert. |
| `extra_config` | object | Extra config carried on the event. (computed) |
| `notify_rule_ids` | []int64 | Notify-rule ids (new notification model). |
| `notify_version` | int | `0` = legacy notify, `1` = notify-rules model. (computed) |
| `notify_rules` | [] | Resolved notify rules, each `{"id":int64,"name":string}`. (computed) |

### `AlertCurEvent` — additional/differing fields

The active-event object (and both `/:eid` responses) shares all fields above, with these differences and additions:

| Field (`json`) | Type | Meaning |
|---|---|---|
| `is_recovered` | **bool** | Differs from his (int): here `true`/`false`. (computed) |
| `trigger_values` | string | Trigger value (mirror of `trigger_value`). (computed) |
| `trigger_values_json` | object | `{"values_with_unit": {name → {value, unit, ...}}}` — formatted trigger values. (computed) |
| `tags_map` | map[string]string | Tags parsed into a key→value map. (computed) |
| `notify_users_obj` | []User | Resolved notify users (notification pipeline). (computed) |
| `last_sent_time` | int64 | Unix seconds of the last notification sent. (computed) |
| `first_eval_time` | int64 | Unix seconds of the first anomaly detection. (computed) |
| `status` | int | Event status flag. (computed) |
| `claimant` | string | Who claimed/acknowledged the event. (computed) |
| `sub_rule_id` | int64 | Sub-rule id (for rules that expand into sub-rules). (computed) |
| `extra_info` | []string | Extra info lines. (computed) |
| `target` | object | Resolved target object (host details), if any. (computed) |
| `recover_config` | object | Recovery configuration for the event. (computed) |
| `rule_hash` | string | Rule hash. (computed) |
| `shot_image_base64` | map[string]string | Screenshot images, base64-encoded. (computed) |
| `extra_info_map` | []map[string]string | Extra info as a list of key→value maps. (computed) |
| `notify_rule_id` | int64 | Single notify-rule id (convenience). (computed) |
| `notify_rule_name` | string | Single notify-rule name (convenience). (computed) |

## Example
Request:
```json
{"method":"GET","path":"/api/n9e/alert-his-events/list","query":{"hours":"24","limit":"100","p":"1"}}
```
Response (trimmed):
```json
{
  "ok": true,
  "status": 200,
  "data": {
    "dat": {
      "total": 342,
      "list": [
        {
          "id": 90271,
          "cate": "prometheus",
          "is_recovered": 0,
          "datasource_id": 1,
          "group_id": 2,
          "group_name": "ops-team",
          "rule_id": 88,
          "rule_name": "Host CPU usage high",
          "rule_prod": "metric",
          "severity": 2,
          "prom_ql": "cpu_usage_idle < 10",
          "target_ident": "host-01",
          "trigger_time": 1751330400,
          "trigger_value": "6.3",
          "recover_time": 0,
          "last_eval_time": 1751330460,
          "tags": ["ident=host-01", "app=web"],
          "annotations": {"summary": "CPU idle 6.3% on host-01"},
          "notify_groups_obj": [{"id": 3, "name": "ops-oncall"}]
        }
      ]
    },
    "err": ""
  }
}
```
