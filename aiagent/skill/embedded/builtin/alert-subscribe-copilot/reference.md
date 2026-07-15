# Full subscription rule config field table

Data model `models/alert_subscribe.go:AlertSubscribe`.

## Basic fields

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | Yes | Subscription rule name |
| `note` | string | No | Notes |
| `disabled` | int | No | `0`=enabled (default), `1`=disabled. A disabled subscription is filtered at the cache layer and does not participate in matching at all |
| `group_id` | int64 | Yes | **Management ownership business group** (determines who can view/edit this subscription); **does not participate in event matching** (the tool can pass it via the `group_id` parameter) |
| `prod` | string | No | Product type. When non-empty, **exact match** against the event's RuleProd; empty = no filter. Common values `"metric"`/`"logging"`/`"host"` |
| `cate` | string | No | Datasource type. **In the current implementation only `"host"` has a real filtering effect** (matches only host events), other values are equivalent to no filtering; empty = no filter |
| `datasource_ids` | int[] | No | Datasource ID list; empty array or containing `0` = all (the tool automatically normalizes to `[0]`); when an event has no datasource (dsId=0), this filter is skipped |
| `cluster` | string | No | Always set to `"0"` (a V5 legacy field) |

## Subscription filter conditions

| Field | Type | Required | Description |
|---|---|---|---|
| `rule_ids` | int[] | No | List of subscribed alert rule IDs; empty = subscribe to events from **all** alert rules (global subscription) |
| `severities` | int[] | **Yes** | Subscribed alert severities, validated as non-empty in both new and old versions; `[1,2,3]` = all |
| `for_duration` | int64 | No | Seconds. Forward only when the alert duration (`trigger_time - first_trigger_time`) **exceeds** this value — used for "alert escalation" (e.g. `300` = forward via the subscription only after persisting unrecovered for 5 minutes). `0` = no limit |
| `tags` | array | No | Event tag filter, multiple entries are **AND**'d, structure as below |
| `busi_groups` | array | No | Filter by the event's **business group name**, multiple entries are **AND**'d. Element `{"key":"groups","func":"=~","value":"production.*"}` — the key is by convention written as `"groups"` (in the implementation the key does not participate in matching; func/value match against the event's GroupName) |

### tags element structure and operators

```json
{ "key": "tag name", "func": "match operator", "value": "match value" }
```

| func | Meaning | value example |
|---|---|---|
| `==` | Exact match | `"web01"` |
| `!=` | Not equal | `"web01"` |
| `=~` | Regex match | `"web.*"` |
| `!~` | Regex not match | `"web.*"` |
| `in` | In list (space-separated) | `"web01 web02 web03"` |
| `not in` | Not in list (space-separated) | `"web01 web02"` |

Common tags: `ident` (machine identifier), `rulename` (alert rule name), `__name__` (metric name), and custom business tags.

### severity alert levels

| Value | Meaning |
|---|---|
| 1 | Level-1 alert (Critical) |
| 2 | Level-2 alert (Warning) |
| 3 | Level-3 alert (Info) |

## Notification config: new version vs. old version

| Field | Type | Description |
|---|---|---|
| `notify_version` | int | `1`=new version (recommended, forwarded via notification rules); `0`=old version (directly fill in user groups + channels) |
| `notify_rule_ids` | int[] | Required non-empty in the new version: the cloned event's notification outlet is rewritten to these notification rules |

**The new-version (notify_version=1) validation clears all old-version fields**: `user_group_ids`, `redefine_channels`/`new_channels`, `redefine_webhooks`/`webhooks`, `redefine_severity`/`new_severity` (`models/alert_subscribe.go:Verify`). In other words, **rewriting severity/channels is an old-version capability**; in the new version, to change severity/channels, do the routing at the notification rule layer (→ notify-rule-copilot).

Old-version (notify_version=0) fields, encountered only when maintaining existing config:

| Field | Description |
|---|---|
| `user_group_ids` | Receiving user group IDs, a **space-separated string** (e.g. `"1 2"`); if you specify it you must also specify `new_channels` |
| `redefine_severity` / `new_severity` | When =1, the cloned event's severity is changed to new_severity |
| `redefine_channels` / `new_channels` | When =1, the cloned event's notification channels are changed to new_channels (space-separated) |
| `redefine_webhooks` / `webhooks` | When =1, the cloned event's callbacks are changed to webhooks (a JSON array). **When not enabled, the cloned event's callbacks are cleared** (to prevent re-hitting the original callbacks) — this also applies to the new version |
| `extra_config` | An extension config JSON object; the engine has no fixed use for it, just copy `{}` |

## Complete examples

### Example 1: Subscribe to all Critical alerts and forward to the on-call notification rule (most common)

```json
{
  "name": "Subscribe to all Critical alerts",
  "note": "CC all critical alerts to the on-call chain",
  "disabled": 0,
  "prod": "",
  "cate": "",
  "datasource_ids": [],
  "cluster": "0",
  "rule_ids": [],
  "severities": [1],
  "for_duration": 0,
  "tags": [],
  "busi_groups": [],
  "extra_config": {},
  "notify_version": 1,
  "notify_rule_ids": [1]
}
```

### Example 2: Subscribe to alerts from specific machines by tag

```json
{
  "name": "Subscribe to web cluster alerts",
  "note": "CC alerts from all web-prefixed machines to the web team",
  "disabled": 0,
  "prod": "metric",
  "cate": "",
  "datasource_ids": [],
  "cluster": "0",
  "rule_ids": [],
  "severities": [1, 2, 3],
  "for_duration": 0,
  "tags": [
    {"key": "ident", "func": "=~", "value": "web.*"}
  ],
  "busi_groups": [],
  "extra_config": {},
  "notify_version": 1,
  "notify_rule_ids": [1]
}
```

### Example 3: Alert escalation — notify the supervisor after a specified rule persists unrecovered for 5 minutes

```json
{
  "name": "CPU alert escalation",
  "note": "CPU-related alerts that persist unrecovered for 5 minutes, escalate to second-line",
  "disabled": 0,
  "prod": "metric",
  "cate": "",
  "datasource_ids": [1],
  "cluster": "0",
  "rule_ids": [10, 11],
  "severities": [1, 2],
  "for_duration": 300,
  "tags": [],
  "busi_groups": [],
  "extra_config": {},
  "notify_version": 1,
  "notify_rule_ids": [2]
}
```

### Example 4: Cross-business-group subscription — receive only database alerts from the "production" business group

```json
{
  "name": "Subscribe to production database alerts",
  "note": "CC database-related alerts under the production business group to the DBA",
  "disabled": 0,
  "prod": "metric",
  "cate": "",
  "datasource_ids": [],
  "cluster": "0",
  "rule_ids": [],
  "severities": [1, 2],
  "for_duration": 0,
  "tags": [
    {"key": "rulename", "func": "=~", "value": ".*database.*|.*MySQL.*|.*Redis.*"}
  ],
  "busi_groups": [
    {"key": "groups", "func": "=~", "value": "production.*"}
  ],
  "extra_config": {},
  "notify_version": 1,
  "notify_rule_ids": [2, 3]
}
```

### Example 5: Aggregate alert events and forward to a ticketing system

```json
{
  "name": "Forward alerts to the ticketing system",
  "note": "Warning-and-above alerts are uniformly forwarded to the internal ticketing system via a notification rule",
  "disabled": 0,
  "prod": "",
  "cate": "",
  "datasource_ids": [],
  "cluster": "0",
  "rule_ids": [],
  "severities": [1, 2],
  "for_duration": 0,
  "tags": [],
  "busi_groups": [],
  "extra_config": {},
  "notify_version": 1,
  "notify_rule_ids": [5]
}
```
