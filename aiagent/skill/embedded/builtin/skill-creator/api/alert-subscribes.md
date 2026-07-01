# Alert subscriptions

Alert subscriptions ("subscribes") are rules that subscribe a user group (or, in the new
notify model, a set of notify rules) to alert events matching a filter — by rule, product,
datasource, severity, tags, and busi-group. A subscription can also **redefine routing** for
the matched events in the legacy model: override severity, notification channels, and
webhooks before the event is delivered.

> Gateway call: GET, `path` relative to `/api/n9e`. Response `{"ok":true,"status":200,"data":{"dat":<payload>,"err":""}}` — read `data["dat"]`. Protocol: see `../n9e-api.md`.

## Endpoints
| Path | Purpose | `dat` shape |
|---|---|---|
| `/busi-groups/alert-subscribes` | Subscriptions across your busi groups (admin sees all). | Pattern B (bare array) |
| `/busi-group/:id/alert-subscribes` | Subscriptions of one busi group (`:id` in path). | Pattern B (bare array) |

## Query parameters
| Param | Type | Required | Default | Meaning | Endpoint |
|---|---|---|---|---|---|
| `gids` | string (csv) | No | all your groups | Comma-separated busi-group ids to filter by; empty = every group you can see. | `/busi-groups/alert-subscribes` |
| — | — | — | — | No query params; group is taken from the `:id` path segment. | `/busi-group/:id/alert-subscribes` |

## Response — `dat` payload
(Pattern B bare array.) Each subscription is an `AlertSubscribe`:

| Field (`json`) | Type | Meaning |
|---|---|---|
| `id` | int64 | Subscription id. |
| `name` | string | Subscription name. |
| `disabled` | int | 0 = enabled, 1 = disabled. |
| `group_id` | int64 | Busi-group id this subscription belongs to. |
| `prod` | string | Product / monitoring type it subscribes (e.g. `metric`, `host`, `anomaly`, `logging`); empty = any. |
| `cate` | string | Datasource category filter (e.g. `prometheus`, `host`); empty = any. |
| `datasource_ids` | []int64 | (computed) Datasource ids the subscription matches; `[0]` means all datasources. |
| `cluster` | string | Legacy: effective clusters, space-separated (superseded by `datasource_ids`). |
| `rule_id` | int64 | Legacy: single subscribed alert-rule id (superseded by `rule_ids`). |
| `severities` | []int | (computed) Subscribed severities to match (1 = Emergency, 2 = Warning, 3 = Notice). |
| `for_duration` | int64 | Only match events that have persisted at least this many seconds (0 = no delay). |
| `rule_name` | string | (computed) Legacy: name of `rule_id`. |
| `tags` | array | Tag matchers on the event's tags — array of `TagFilter` (see below); all must match. |
| `redefine_severity` | int | 0/1 — whether to override the event's severity with `new_severity`. |
| `new_severity` | int | New severity applied when `redefine_severity` = 1 (1/2/3). |
| `redefine_channels` | int | 0/1 — whether to override the event's notify channels with `new_channels`. |
| `new_channels` | string | New notify channels (space-separated) applied when `redefine_channels` = 1. |
| `user_group_ids` | string | Legacy notify: space-separated user-group ids to notify. |
| `user_groups` | []UserGroup | (computed) Resolved user-group objects for `user_group_ids`. |
| `redefine_webhooks` | int | 0/1 — whether to override the event's webhooks with `webhooks`. |
| `webhooks` | []string | (computed) Webhook URLs applied when `redefine_webhooks` = 1. |
| `extra_config` | any | (computed) Arbitrary extra configuration (parsed from stored JSON). |
| `note` | string | Free-text description. |
| `create_by` | string | Creator username. |
| `create_at` | int64 | Create time (unix seconds). |
| `update_by` | string | Last updater username. |
| `update_at` | int64 | Last update time (unix seconds). |
| `update_by_nickname` | string | (computed) Display nickname of `update_by`. |
| `busi_groups` | array | Busi-group matchers — array of `TagFilter` (see below) filtering which busi groups' events match. |
| `rule_ids` | []int64 | Subscribed alert-rule ids (v6; replaces `rule_id`). Empty = match any rule. |
| `notify_rule_ids` | []int64 | New notify model: notify-rule ids used when `notify_version` = 1. |
| `notify_version` | int | 0 = legacy notify (user groups + channels/webhooks overrides), 1 = new notify-rule model (`notify_rule_ids`). |
| `rule_names` | []string | (computed) Names for each id in `rule_ids`. |

### `TagFilter` sub-type (used by `tags` and `busi_groups`)
Each matcher object has these json fields:

| Field (`json`) | Type | Meaning |
|---|---|---|
| `key` | string | Tag key to match (for `busi_groups` the key is typically `group_name`). |
| `func` | string | Match operator: `==`, `=~`, `in`, `!=`, `!~`, `not in`. |
| `op` | string | Alias of `func` (fallback when `func` is empty). |
| `value` | any | Value to compare against (string, or list/regex depending on `func`). |

## Example
Request:
```json
{"method":"GET","path":"/busi-groups/alert-subscribes","query":{}}
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
        "name": "escalate disk alerts to oncall",
        "disabled": 0,
        "group_id": 2,
        "prod": "metric",
        "cate": "prometheus",
        "datasource_ids": [0],
        "cluster": "",
        "rule_id": 0,
        "severities": [1, 2],
        "for_duration": 0,
        "rule_name": "",
        "tags": [
          {"key": "app", "func": "==", "op": "==", "value": "mysql"}
        ],
        "redefine_severity": 1,
        "new_severity": 1,
        "redefine_channels": 0,
        "new_channels": "",
        "user_group_ids": "3 5",
        "user_groups": [{"id": 3, "name": "oncall"}],
        "redefine_webhooks": 0,
        "webhooks": [],
        "extra_config": null,
        "note": "",
        "create_by": "root",
        "create_at": 1719800000,
        "update_by": "root",
        "update_at": 1719803600,
        "update_by_nickname": "Root",
        "busi_groups": [],
        "rule_ids": [101, 102],
        "notify_rule_ids": [],
        "notify_version": 0,
        "rule_names": ["disk usage high", "disk will fill in 24h"]
      }
    ],
    "err": ""
  }
}
```
