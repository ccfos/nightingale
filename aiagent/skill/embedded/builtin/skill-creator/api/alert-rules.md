# Alert rules

Alerting rule definitions: what condition fires an event, on which datasource, at what
severity, and how the alert is routed to notifications. Use these endpoints to read rule
definitions, audit coverage, or summarize how alerting is configured across business groups.

> Gateway call: GET. Include the `/api/n9e` prefix in `path`. Response `{"ok":true,"status":200,"data":{"dat":<payload>,"err":""}}` — read `data["dat"]`. Protocol: see `../n9e-api.md`.

## Endpoints
| Path | Purpose | `dat` shape |
|---|---|---|
| `/busi-groups/alert-rules` | Rules across the groups your RBAC allows. | Pattern B — bare array of `AlertRule` |
| `/busi-group/:id/alert-rules` | Rules of ONE business group (`:id` in path). | Pattern B — bare array of `AlertRule` |
| `/alert-rule/:arid` | One rule's full definition (`:arid` in path). | single `AlertRule` object |

## Query parameters
| Param | Type | Required | Default | Meaning | Endpoint |
|---|---|---|---|---|---|
| `gids` | string (csv of group ids) | no | empty = your groups (all groups for admin) | Restrict to specific business group ids, e.g. `"1,2"`. | `/busi-groups/alert-rules` |
| `stime` | string (unix secs) | no | — | Start of a recent-window used to attach a per-rule event count. | `/busi-groups/alert-rules` |
| `etime` | string (unix secs) | no | — | End of that recent-window. Pass together with `stime`. | `/busi-groups/alert-rules` |

`/busi-group/:id/alert-rules` and `/alert-rule/:arid` take no query params (the id is in the path).
Stringify all query values (see `../n9e-api.md`).

## Response — `dat` payload
Pattern B (bare array) for the two list endpoints; a single object for `/alert-rule/:arid`.
Each rule is an `AlertRule` (from `models/alert_rule.go`):

| Field (`json`) | Type | Meaning |
|---|---|---|
| `id` | int64 | Rule id. |
| `group_id` | int64 | Business group id this rule belongs to. |
| `cate` | string | Datasource category, e.g. `prometheus`, `elasticsearch`, `mysql`, `host`, `tdengine`, `ck`. Drives the shape of `rule_config`. |
| `datasource_ids` | []int64 | (computed) Resolved datasource ids the rule targets (list-page convenience field; omitted when empty). |
| `datasource_queries` | []DatasourceQuery | Datasource selectors: each is `{match_type, op, values}` (how to match which datasources apply). |
| `cluster` | string | Deprecated (use `datasource_queries`). Legacy space-separated cluster list. |
| `name` | string | Rule name. |
| `note` | string | Free-text note; included in notifications. |
| `prod` | string | Product line; empty means core n9e. |
| `algorithm` | string | Detection algorithm; empty = threshold, `holtwinters` = anomaly. |
| `algo_params` | interface{} | (computed) Parameters for the algorithm (FE-facing form of the stored params). |
| `delay` | int | Seconds to delay evaluation. |
| `severity` | int | 1 = Emergency, 2 = Warning, 3 = Notice. |
| `severities` | []int | (computed) Multiple severities when a rule emits more than one level. |
| `disabled` | int | 0 = enabled, 1 = disabled. |
| `prom_for_duration` | int | Deprecated (use `cron_pattern`). Prometheus `for` duration, seconds. |
| `prom_ql` | string | Single PromQL expression (simple prometheus rules). |
| `rule_config` | interface{} | (computed) The detection config (queries/thresholds/triggers). Big nested JSON whose structure varies by datasource category `cate` — do not assume a fixed schema. |
| `event_relabel_config` | []*RelabelConfig | (computed) Relabel rules applied to generated events. |
| `prom_eval_interval` | int | Evaluation interval, seconds. |
| `enable_stime` | string | (computed) Deprecated. Single effective-window start "HH:MM" (FE). |
| `enable_stimes` | []string | (computed) Effective-window start times (FE). |
| `enable_etime` | string | (computed) Deprecated. Single effective-window end "HH:MM" (FE). |
| `enable_etimes` | []string | (computed) Effective-window end times (FE). |
| `enable_days_of_week` | []string | (computed) Deprecated. Active weekdays (FE). |
| `enable_days_of_weeks` | [][]string | (computed) Active weekdays per window (FE). |
| `enable_in_bg` | int | 0 = global, 1 = only within the one business group. |
| `notify_recovered` | int | Whether to notify on recovery (1) or not (0). |
| `notify_channels` | []string | (computed) Deprecated. Legacy channel list (sms/voice/email/dingtalk/…). |
| `notify_groups_obj` | []UserGroup | (computed) Deprecated. Resolved notify user-group objects (FE). |
| `notify_groups` | []string | (computed) Deprecated. Notify user-group ids (FE). |
| `notify_repeat_step` | int | Repeat-notify interval, minutes. |
| `notify_max_number` | int | Max number of repeat notifications. |
| `recover_duration` | int64 | Seconds a condition must stay clear before it counts as recovered. |
| `callbacks` | []string | (computed) Deprecated. Legacy callback URLs (FE). |
| `runbook_url` | string | Runbook / SOP URL. |
| `append_tags` | []string | (computed) Tags appended to events, e.g. `service=n9e` (FE). |
| `annotations` | map[string]string | (computed) Extra annotations attached to events (FE). |
| `extra_config` | interface{} | (computed) Miscellaneous extra config (FE). |
| `create_at` | int64 | Creation time, unix seconds. |
| `create_by` | string | Creator username. |
| `update_at` | int64 | Last-update time, unix seconds. |
| `update_by` | string | Last-updater username. |
| `uuid` | int64 | (computed) Template identifier. |
| `cur_event_count` | int64 | (computed) Recent-event count attached when `stime`/`etime` are passed to the list endpoint. |
| `update_by_nickname` | string | (computed) Display nickname of the last updater (FE). |
| `cron_pattern` | string | Cron expression controlling when the rule evaluates. |
| `time_zone` | string | Timezone for evaluation, e.g. `Asia/Shanghai`, `UTC`; empty = default. |
| `notify_rule_ids` | []int64 | Ids of notification rules used for routing (new notify system). |
| `pipeline_configs` | []PipelineConfig | Event-pipeline configs bound to this rule. |
| `notify_version` | int | Notification system version: 0 = old, 1 = new. |

## Example
Request:
```json
{"method":"GET","path":"/api/n9e/busi-groups/alert-rules","query":{"gids":"1,2"}}
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
        "group_id": 1,
        "cate": "prometheus",
        "datasource_queries": [{"match_type": 0, "op": "in", "values": [3]}],
        "name": "Host CPU high",
        "note": "cpu usage over 90% for 5m",
        "prod": "",
        "algorithm": "",
        "severity": 2,
        "disabled": 0,
        "prom_ql": "",
        "rule_config": { "queries": [], "triggers": [] },
        "prom_eval_interval": 15,
        "notify_recovered": 1,
        "notify_repeat_step": 60,
        "recover_duration": 0,
        "runbook_url": "",
        "append_tags": ["team=infra"],
        "cron_pattern": "",
        "time_zone": "Asia/Shanghai",
        "notify_rule_ids": [4],
        "notify_version": 1,
        "cur_event_count": 3,
        "create_by": "root",
        "create_at": 1719800000,
        "update_by": "root",
        "update_at": 1719805000
      }
    ],
    "err": ""
  }
}
```
