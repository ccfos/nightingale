# Troubleshooting "mute not working" + behavior semantics

## "I muted it but it's still alerting" troubleshooting chain

The engine match function `alert/mute/mute.go:MatchMute` judges in the following order, and **if any gate fails, the event is not muted**. Check in order:

| # | Check item | Common reasons it fails |
|---|---|---|
| 0 | Cache sync | Just changed a rule; the in-memory cache lags by at most **9 seconds**; expired fixed-period mutes are not loaded into the cache |
| 1 | Business group | Muting is isolated by `group_id`; **the event's business group and the mute rule are not in the same business group** (one of the most common root causes) |
| 2 | `disabled` | The rule is disabled (`disabled=1`) |
| 3 | Datasource | `datasource_ids` is not all (does not contain 0) and the event's `datasource_id` is not in the list |
| 4 | Time | Fixed period: the event trigger time is not in the closed interval `[btime, etime]`; periodic: the weekday or HH:mm period did not match (uses the n9e process's local timezone; 0=Sunday) |
| 5 | Severity | `severities` is non-empty and the event severity is not in it |
| 6 | Tags | Multiple `tags` are AND'd; if the event is missing any one tag or value mismatch, it is not muted; writing the `in` value as comma-separated will also mismatch (must be spaces) |

Also note: muting only blocks **newly evaluated events**. If an event was already produced and the active alert is still hanging there → muting will not clear it (see behavior semantics below).

## Behavior semantics (cite when answering user questions)

| Behavior | Semantics |
|---|---|
| Where do muted events go | Dropped directly during the evaluation stage (`alert/process/process.go`): **not persisted (no active/historical record), no notification sent**; only leaves a trace in the server log and the mute count metric |
| Will it falsely recover during the mute | No. The hash of a muted event is still recorded into the alerting set, so an already-triggered alert will not be judged as "recovered" because of muting |
| Does muting affect existing active alerts | No. Active alerts produced before muting continue to hang in the list; after muting takes effect, they no longer receive new notifications (new events are blocked), but the records are not cleared |
| Are expired mutes deleted automatically | **No.** Expired fixed-period mutes remain in the DB (the frontend can query them with `expired=1`); they are just no longer loaded into the matching cache. Admins can clean up in the background via the bulk cleanup API (`DELETE /api/n9e/alert-mutes`); periodic mutes are not cleaned up |
| `Activated` field | A frontend display field indicating "whether it is within the effective period at this moment", computed at query time, not a stored state |
| How long until a change takes effect | The cache polls the DB stats every 9 seconds and reloads only when there is a change; **at most 9 seconds**, no restart needed |
| Do prod/cate/cluster participate in matching | **No**, stored for display only |

## Other gotchas

| Symptom | Cause | Handling |
|---|---|---|
| `etime <= btime` error | Verify's hard validation, required for periodic mutes too | Use the tool's `duration` parameter, or omit btime/etime and let the tool fill them in automatically |
| Behavior at the periodic-mute btime/etime boundary is unexpected | The current implementation's periodic matching **does not look at** btime/etime, only the weekday + time period | To get a "periodic mute that only takes effect in a certain month", you currently have to manually disable/delete it when it expires |
| `enable_days_of_week` written as `1-5` or `Monday to Friday` does not work | The engine does a contains match on the space-separated digit string | Write `"1 2 3 4 5"`, or use a tool-recognized alias such as `"weekday"` |
| tags has `key` but `value` left empty | An empty value participates in exact matching and mostly mismatches | Confirm the real tag value; when unsure, first look at the tags of one real event |
| Want to mute "a certain alert rule" | The event tags contain `rulename` | Use `{"key":"rulename","func":"==","value":"<rule name>"}`; remember to sync after the rule name changes |
| Large-scope accidental muting | An empty tags array = mute all alerts within the business group | Confirm with the user before creating; when troubleshooting, first check whether an "unconditional mute" rule is in effect |

## Verification methods

- In-app: `get_alert_mute_detail` to verify fields; `list_alert_mutes` to check whether other mute rules in the same business group interfere with the judgment.
- HTTP (give the user commands): `POST /api/n9e/busi-group/:id/alert-mutes/preview` previews the active events that would be hit by the mute; `POST /api/n9e/alert-mute-tryrun` does a trial run with a rule draft. See `http-api.md`.
