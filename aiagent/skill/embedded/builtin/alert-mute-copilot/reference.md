# Full table of mute rule config fields

Data model `models/alert_mute.go:AlertMute`.

## Basic fields

| Field | Type | Required | Description |
|---|---|---|---|
| `group_id` | int64 | Yes | ID of the owning business group (the tool can take it via the `group_id` parameter) |
| `note` | string | Yes | Mute rule name/title |
| `cause` | string | No | Reason for muting |
| `prod` | string | No | Product type, defaults to `"metric"`. **Stored for display only, does not participate in matching** |
| `cate` | string | No | Datasource type, e.g. `"prometheus"`. **Stored for display only, does not participate in matching** |
| `cluster` | string | No | Fixed to `"0"` (legacy V5 field) |
| `datasource_ids` | int[] | No | List of datasource IDs; an empty array or one containing `0` = all datasources (the tool/Verify automatically normalizes to `[0]`) |
| `severities` | int[] | Recommended | Alert severities to mute; `[1,2,3]` = all. Engine semantics: when the list is non-empty, the event severity must be in it |
| `tags` | array | No | Tag match conditions, multiple conditions **AND'd**; **empty array = mute all alerts within the business group** |
| `mute_time_type` | int | Yes | `0`=fixed time range, `1`=periodic mute |
| `disabled` | int | No | `0`=enabled (default), `1`=disabled. A disabled rule does not participate in matching at all |

### severity alert level

| Value | Meaning |
|---|---|
| 1 | Level-1 alarm (Critical) |
| 2 | Level-2 alarm (Warning) |
| 3 | Level-3 alarm (Info) |

## Tag filtering (tags)

```json
{ "key": "tag name", "func": "match operator", "value": "match value" }
```

| func | Meaning | value example |
|---|---|---|
| `==` | Exact match | `"web01"` |
| `!=` | Not equal | `"web01"` |
| `=~` | Regex match | `"web.*"` |
| `!~` | Regex non-match | `"web.*"` |
| `in` | In the list (space-separated) | `"web01 web02 web03"` |
| `not in` | Not in the list (space-separated) | `"web01 web02"` |

- **Multiple tags are AND'd**: an event must match all conditions simultaneously to be muted (`alert/common/key.go:MatchTags`).
- The value for `in`/`not in` can be written either as a space-separated string or passed directly as an array (e.g. `["web01","web02"]`); the tool automatically normalizes it to space-separated.
- `key` cannot be empty; `func` must be one of the six in the table above (the tool validates before persisting).

Common tags: `ident` (machine identifier/hostname), `rulename` (alert rule name), `__name__` (metric name), custom business tags.

## Mute time configuration

### Mode 1: Fixed time range (mute_time_type: 0)

Mutes within the closed interval `[btime, etime]` (judged by the event trigger time). **Prefer expressing the duration via the tool's `duration` parameter rather than computing timestamps yourself.**

| Field | Type | Description |
|---|---|---|
| `mute_time_type` | int | Fixed `0` |
| `duration` (**tool parameter**, not a config field) | string | Mute duration: `"2h"`/`"30m"`/`"7d"`/`"1d12h"`/`"1w"` (supports s/m/h/d/w and combinations). If passed, you do not need to fill in btime/etime |
| `btime` | int64 | Start time, Unix seconds; **omit to default to the current time** |
| `etime` | int64 | End time, Unix seconds; not needed when `duration` is used (the tool computes it as `btime+duration`). **`etime > btime` is a hard validation** |
| `periodic_mutes` | array | Leave empty `[]` |

> Only fill in `btime`/`etime` yourself when the user gives **absolute start/end moments** (rather than "how long to mute"). The system prompt's `Now:` line already provides the current precise time and Unix seconds.

### Mode 2: Periodic mute (mute_time_type: 1)

Mutes periodically by day of week or day of month, within a time period.

| Field | Type | Description |
|---|---|---|
| `mute_time_type` | int | Fixed `1` |
| `btime` / `etime` | int64 | **Can be omitted** (the tool fills them in automatically as the current time / one year later). Note: **the current implementation's periodic matching only looks at the days + time period in periodic_mutes; btime/etime do not participate in the judgment**, they only exist to pass the `etime > btime` validation |
| `periodic_mutes` | array | Periodic configuration, can be multiple groups (a mute occurs if any group matches, OR); weekly and monthly groups can be mixed |

`periodic_mutes` element structure, each element is either weekly or monthly:

```json
{"enable_days_of_week": "1 2 3 4 5", "enable_stime": "02:00", "enable_etime": "06:00"}
{"cycle": "month", "enable_days_of_month": [1, 15, -1], "enable_stime": "00:00", "enable_etime": "06:00"}
```

| Field | Description |
|---|---|
| `cycle` | `"week"` (default when omitted) or `"month"`. **Must be `"month"` for monthly groups**; otherwise `enable_days_of_month` is ignored and the group matches no day |
| `enable_days_of_week` | Weekly only. Effective days of week, space-separated, **0=Sunday…6=Saturday**. You can also write the aliases `"weekday"`/`"everyday"`/`"weekend"` directly, and the tool converts them automatically |
| `enable_days_of_month` | Monthly only. Integer array, `1`-`31` for the day of month, `-1` for the last day of every month. A day missing from a month (e.g. 31 in April) is skipped that month, so use `-1` for "month end" |
| `enable_stime` / `enable_etime` | Daily effective period `HH:mm`. Writing `"allday"` (or 24h) = 00:00~23:59; `stime == etime` is also treated as all-day |

- **Weekly groups support cross-midnight natively**: `stime > etime` (e.g. `22:00`~`06:00`) is judged as "`>= stime` or `< etime`" on the selected days, no need to split into two segments.
- **Monthly groups cannot cross midnight** (rejected on save): split into two groups. An end of `00:00` means the end of that day, so "last day 22:00 ~ next day 06:00" is `-1` with `22:00`~`00:00` plus `1` with `00:00`~`06:00` (see Example 5). Do not use `23:59` as the end, which leaves the last minute unmuted.
- Time judgment uses the local timezone of the n9e process.

## Complete examples

### Example 1: Fixed-time mute of a specific machine (2 hours)

> When calling, pass the tool parameter `duration: "2h"`; btime/etime are not needed in config.

```json
{
  "note": "Maintenance window: mute web01 alerts",
  "cause": "web01 planned maintenance, expected 2 hours",
  "prod": "metric",
  "cate": "prometheus",
  "datasource_ids": [],
  "severities": [1, 2, 3],
  "tags": [
    {"key": "ident", "func": "==", "value": "web01"}
  ],
  "mute_time_type": 0,
  "periodic_mutes": [],
  "cluster": "0"
}
```

### Example 2: Fixed-time mute of specific alerts on multiple machines (1 day)

> When calling, pass the tool parameter `duration: "1d"`.

```json
{
  "note": "Batch maintenance: mute web cluster CPU alerts",
  "cause": "web cluster upgrade maintenance",
  "prod": "metric",
  "cate": "prometheus",
  "datasource_ids": [],
  "severities": [2, 3],
  "tags": [
    {"key": "ident", "func": "in", "value": "web01 web02 web03"},
    {"key": "rulename", "func": "=~", "value": ".*CPU.*"}
  ],
  "mute_time_type": 0,
  "periodic_mutes": [],
  "cluster": "0"
}
```

### Example 3: Periodic mute (every day from 2-6 AM)

```json
{
  "note": "Routine maintenance window: early-morning mute",
  "cause": "Mute alerts during the daily early-morning batch jobs",
  "prod": "metric",
  "cate": "prometheus",
  "datasource_ids": [],
  "severities": [2, 3],
  "tags": [
    {"key": "ident", "func": "=~", "value": "batch-.*"}
  ],
  "mute_time_type": 1,
  "periodic_mutes": [
    {
      "enable_days_of_week": "0 1 2 3 4 5 6",
      "enable_stime": "02:00",
      "enable_etime": "06:00"
    }
  ],
  "cluster": "0"
}
```

### Example 4: Mute low-severity alerts during the weekday lunch break

```json
{
  "note": "Weekday lunch-break mute",
  "cause": "Reduce non-urgent alert disturbance during the lunch break",
  "prod": "metric",
  "cate": "prometheus",
  "datasource_ids": [],
  "severities": [3],
  "tags": [],
  "mute_time_type": 1,
  "periodic_mutes": [
    {
      "enable_days_of_week": "weekday",
      "enable_stime": "12:00",
      "enable_etime": "13:30"
    }
  ],
  "cluster": "0"
}
```

> Note: in Example 4, empty tags = mute **all** Info-level alerts within the business group; confirm the impact scope with the user before persisting.

### Example 5: Monthly mute across midnight (last day of every month 22:00 ~ next day 06:00)

```json
{
  "note": "Month-end billing window",
  "cause": "Mute billing job alerts during the month-end settlement",
  "prod": "metric",
  "cate": "prometheus",
  "datasource_ids": [],
  "severities": [2, 3],
  "tags": [
    {"key": "service", "func": "==", "value": "billing"}
  ],
  "mute_time_type": 1,
  "periodic_mutes": [
    {"cycle": "month", "enable_days_of_month": [-1], "enable_stime": "22:00", "enable_etime": "00:00"},
    {"cycle": "month", "enable_days_of_month": [1], "enable_stime": "00:00", "enable_etime": "06:00"}
  ],
  "cluster": "0"
}
```
