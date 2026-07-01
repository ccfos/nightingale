# Troubleshooting a subscription not taking effect + behavioral semantics

## "I subscribed but didn't receive anything" troubleshooting chain

Engine matching chain `alert/dispatch/dispatch.go:handleSub`, evaluated in the following order; **failing any single gate skips that subscription**. Check in order:

| # | Check item | Common reason for failure |
|---|---|---|
| 0 | Cache sync | Just changed the rule; the in-memory cache lags by **9 seconds** at most |
| 1 | `disabled` | The rule is disabled (`disabled=1`, filtered directly at the cache layer) |
| 2 | Datasource | `datasource_ids` is not "all" and the event's `datasource_id` is not in the list |
| 3 | prod | `prod` is non-empty and not equal to the event's RuleProd (note this is an exact match) |
| 4 | cate | `"host"` was filled in but the event is not a host type (other values do not cause a mismatch) |
| 5 | Tags | Multiple `tags` entries are AND'd; writing the `in` value comma-separated causes a mismatch (it must be space-separated) |
| 6 | Business group name | `busi_groups` matches against the event's **business group name** — a condition hard-bound by name loses its target after the business group is renamed; the regex did not account for the full name |
| 7 | Duration | `for_duration` is greater than the event's elapsed duration (`trigger_time - first_trigger_time`) — when an alert has just fired it will definitely not match; you must wait for the **next** notification cycle once it has persisted long enough |
| 8 | Severity | `severities` does not include the event's severity |
| 9 | Downstream outlet | All of the above passed and the cloned event was produced, but the **notification rule itself** that `notify_rule_ids` points to is not configured correctly (channel/recipient/applicable attributes do not match) → hand off to notify-rule-copilot to troubleshoot |

Also confirm the source: **the original alert event must actually be produced** — a subscription does not conjure things out of nothing; if the event is muted (intercepted at the evaluation stage) it never reaches the subscription step at all.

## Behavioral semantics (cite when answering user questions)

| Behavior | Semantics |
|---|---|
| Does a subscription replace the original notification | **No**. The original event still travels its own notification path as usual; the subscription clones a copy and forwards it additionally. If the same person is configured on both chains they will receive it twice — this is by design |
| Will it re-hit callbacks (webhooks) | No. The cloned event's callbacks are **cleared** by default (`models/alert_subscribe.go:ModifyEvent`); only the old version with explicit `redefine_webhooks=1` carries the subscription's own webhooks |
| Is the subscription scope limited by group_id | No. `group_id` is only the management ownership (permissions); a subscription inherently receives events from all business groups, and you narrow it via `busi_groups`/`rule_ids`/`tags` |
| How does `for_duration` take effect | It compares the event's `trigger_time - first_trigger_time`. When the event first fires the difference is 0, so it definitely does not match; only after the alert has repeatedly notified to the Nth time and the difference exceeds the threshold is the clone of that event forwarded — so "escalation" depends on the alert rule itself having repeated notifications configured |
| Can the new version rewrite severity/channels | No. When `notify_version=1`, Verify clears the redefine_* fields; do severity/channel routing at the notification rule layer |
| Are recovery events also subscribed | They go through the same matching chain; whether the recovery notification is sent depends on the downstream notification rule's config (e.g. the `is_recovered` attribute filter) |
| How long until changes take effect | The cache polls every 9 seconds, 9 seconds at most, no restart needed |
| How to tell whether an event was forwarded by a subscription | The cloned event carries `sub_rule_id` (the subscription rule ID), visible in the notification record / event detail |

## Other gotchas

| Symptom | Cause | Handling |
|---|---|---|
| Creation reports `severities is required` | Required in both new and old versions | Fill in at least `[1,2,3]` |
| Creation reports `no notify rules selected` | `notify_version=1` but `notify_rule_ids` is empty | First use `list_notify_rules` to get IDs; if there are none, create a notification rule first |
| Creation reports `new_channels is required` | The old version specified `user_group_ids` but did not fill in `new_channels` | Add `new_channels`, or switch to the new version |
| The redefine fields are all gone after saving | The new-version Verify proactively clears the old-version fields | Expected behavior, not data loss |
| The escalation subscription never triggers | The alert rule has no repeated notifications configured (notify_repeat_step=0), so the event will not come a second time | Have the user check the alert rule's repeated-notification interval |
| Event volume from a global subscription explodes | rule_ids/tags/busi_groups all empty = copies all events | Add at least one filter layer; or narrow it at the downstream notification rule |
| The subscription loses its target after a business group rename | `busi_groups` matches by name | Switch to a `=~` regex, or sync the subscription when renaming |

## Verification methods

- In-app: `get_alert_subscribe_detail` to verify fields; `get_notify_rule_detail` to verify the downstream outlet.
- HTTP (give the user commands): `POST /api/n9e/alert-subscribe/alert-subscribes-tryrun` runs a trial using a historical event ID + subscription draft, showing at which step the match failed; the new version even runs the notification rule's real test send. See `http-api.md`.
- Real verification: trigger a matching alert, then go to `Historical Alerts → Detail` to check whether a cloned event carrying `sub_rule_id` and its notification record appear.
