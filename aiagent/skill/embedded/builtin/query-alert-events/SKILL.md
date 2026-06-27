---
name: query-alert-events
description: Query alert events in a Nightingale (n9e) environment. Use this when the user asks to view alerts, query active alerts, search historical alerts, view alert details, or count alert events.
tags:
  - internal
---

# Nightingale (n9e) Query Alert Events

Query and view alert events on the Nightingale monitoring platform. Supports querying current active alerts (not yet recovered), historical alerts (recovered/not recovered), and retrieving the detailed information of a single alert.

---

## Prerequisites

The user must provide:
- **n9e address**: e.g. `http://<n9e-host>:<port>`
- **Username/password**: e.g. `<username>/<password>`
- **Query requirement description**: e.g. "what level-1 alerts occurred in the last hour", "view active alerts", "details of alert ID 123"

If the user has not provided the information above, use the AskUserQuestion tool to ask.

---

## Execution Steps

### Step 1: Log in to obtain a Token

```
POST /api/n9e/auth/login
Content-Type: application/json
Body: {"username":"<username>","password":"<password>"}
```

Extract `dat.access_token` from the response, and include `Authorization: Bearer <token>` in all subsequent requests.

### Step 2: Choose the query type based on the user's requirement

Determine which query method to use based on the user's intent:

- **View active alerts** (alerts currently not recovered) → use the active alerts query API
- **View historical alerts** (including recovered and not recovered) → use the historical alerts query API
- **View alert details** (the complete information of a specific alert) → use the alert details API

Decision rules:
- The user mentions "active", "current", "not recovered", "currently alerting" → active alerts
- The user mentions "historical", "past", "recovered", "has recovered" → historical alerts
- The user mentions a specific alert ID → alert details
- When not explicitly specified, query active alerts by default

### Step 3: Execute the query

#### Method 1: Query active alerts

```
GET /api/n9e/alert-cur-events/list?<query_params>
Authorization: Bearer <token>
```

#### Method 2: Query historical alerts

```
GET /api/n9e/alert-his-events/list?<query_params>
Authorization: Bearer <token>
```

#### Method 3: Query alert details

Active alert details:

```
GET /api/n9e/alert-cur-event/<event_id>
Authorization: Bearer <token>
```

Historical alert details:

```
GET /api/n9e/alert-his-event/<event_id>
Authorization: Bearer <token>
```

### Step 4: Format the output

Present the query results to the user in a readable Markdown table or list, including the key information: alert name, severity, trigger time, duration, trigger value, tags, etc.

---

## Query Parameter Reference

### Active alerts query parameters (alert-cur-events/list)

| Parameter | Type | Required | Default | Description |
|---|---|---|---|---|
| `severity` | string | No | All | Alert severity, comma-separated, e.g. `"1,2"` |
| `query` | string | No | Empty | Search keyword, matches rule name or tags |
| `stime` | int64 | No | No limit | Start time, Unix timestamp (seconds) |
| `etime` | int64 | No | No limit | End time, Unix timestamp (seconds) |
| `hours` | int64 | No | 0 | Last N hours (alternative to stime/etime) |
| `limit` | int | No | 20 | Items per page |
| `p` | int | No | 1 | Page number |
| `bgid` | int64 | No | 0 | Filter by business group ID |
| `rid` | int64 | No | 0 | Filter by alert rule ID |
| `datasource_ids` | string | No | All | Datasource IDs, comma-separated |
| `prods` | string | No | All | Product type, comma-separated, e.g. `"metric,host"` |
| `cate` | string | No | `$all` | Datasource category, comma-separated, e.g. `"prometheus,host"` |
| `my_groups` | bool | No | false | Only show alerts from your own business groups |
| `event_ids` | string | No | Empty | Specific event IDs, comma-separated |

### Historical alerts query parameters (alert-his-events/list)

| Parameter | Type | Required | Default | Description |
|---|---|---|---|---|
| `severity` | int | No | -1 | Alert severity, single value, -1 means all |
| `is_recovered` | int | No | -1 | Recovery status: 0=not recovered, 1=recovered, -1=all |
| `query` | string | No | Empty | Search keyword, matches rule name or tags |
| `stime` | int64 | No | No limit | Start time, Unix timestamp (seconds) |
| `etime` | int64 | No | No limit | End time, Unix timestamp (seconds) |
| `hours` | int64 | No | 0 | Last N hours (alternative to stime/etime) |
| `limit` | int | No | 20 | Items per page |
| `p` | int | No | 1 | Page number |
| `bgid` | int64 | No | 0 | Filter by business group ID |
| `rid` | int64 | No | 0 | Filter by alert rule ID |
| `datasource_ids` | string | No | All | Datasource IDs, comma-separated |
| `prods` | string | No | All | Product type, comma-separated |
| `cate` | string | No | `$all` | Datasource category, comma-separated |

### severity alert levels

| Value | Meaning |
|---|---|
| 1 | Level-1 alert (Critical) |
| 2 | Level-2 alert (Warning) |
| 3 | Level-3 alert (Info) |

### Time parameter usage

There are two ways to specify a time range (choose one):

**Method 1**: Use the `hours` parameter (recommended, simpler)
- `hours=1` → last 1 hour
- `hours=6` → last 6 hours
- `hours=24` → last 24 hours
- `hours=168` → last 7 days

**Method 2**: Use the `stime` + `etime` parameters (precise control)
- Both are Unix timestamps (seconds)
- When only `stime` is passed, `etime` is automatically set to the current time + 24 hours

---

## Response Data Structure

### List response

```json
{
  "dat": {
    "total": 42,
    "list": [<alert event object>, ...]
  }
}
```

### Active alert event fields

```json
{
  "id": 12345,
  "rule_id": 10,
  "rule_name": "CPU usage too high",
  "rule_note": "CPU usage exceeded 80% for 5 minutes",
  "rule_prod": "metric",
  "severity": 1,
  "cate": "prometheus",
  "cluster": "default",
  "datasource_id": 1,
  "group_id": 2,
  "group_name": "Production environment",
  "hash": "rule_10_xxxx",
  "target_ident": "web-server-01",
  "target_note": "Web server",
  "first_trigger_time": 1712000000,
  "trigger_time": 1712003600,
  "last_eval_time": 1712003600,
  "trigger_value": "85.6",
  "prom_ql": "cpu_usage_active{ident=\"web-server-01\"}",
  "prom_eval_interval": 30,
  "prom_for_duration": 300,
  "tags": ["ident=web-server-01", "cpu=cpu-total"],
  "annotations": {"description": "web-server-01 CPU high"},
  "notify_version": 1,
  "notify_channels": [],
  "notify_groups_obj": [],
  "notify_rules": [{"id": 1, "name": "Ops notification"}],
  "callbacks": [],
  "runbook_url": ""
}
```

### Additional fields for historical alert events

Historical alert events add the following on top of active alerts:

```json
{
  "is_recovered": 1,
  "recover_time": 1712007200
}
```

| is_recovered value | Meaning |
|---|---|
| 0 | Not recovered (alert still triggering) |
| 1 | Recovered |

---

## Auxiliary Query APIs

### Get available event tag keys

```
GET /api/n9e/event-tagkeys
Authorization: Bearer <token>
```

Returns the list of tag keys that can be used for search filtering.

### Get tag values

```
GET /api/n9e/event-tagvalues?key=<tag_key>
Authorization: Bearer <token>
```

Returns the top 20 most frequent values for the specified tag key.

### Get datasources associated with alert events

```
GET /api/n9e/alert-cur-events-datasources
Authorization: Bearer <token>
```

### Get the business group list (for bgid filtering)

```
GET /api/n9e/busi-groups
Authorization: Bearer <token>
```

---

## Common Query Examples

### Example 1: View all active alerts

```
GET /api/n9e/alert-cur-events/list?limit=50
```

### Example 2: View level-1 alerts in the last hour

```
GET /api/n9e/alert-cur-events/list?severity=1&hours=1
```

### Example 3: Search active alerts by keyword

```
GET /api/n9e/alert-cur-events/list?query=CPU&limit=20
```

### Example 4: View historical alerts in the last 24 hours (recovered only)

```
GET /api/n9e/alert-his-events/list?hours=24&is_recovered=1&limit=50
```

### Example 5: View active alerts for a specific business group

```
GET /api/n9e/alert-cur-events/list?bgid=2&limit=50
```

### Example 6: View events triggered by a specific alert rule

```
GET /api/n9e/alert-cur-events/list?rid=10&limit=50
```

### Example 7: Combined filtering (level-1 and level-2 alerts + keyword + time range)

```
GET /api/n9e/alert-cur-events/list?severity=1,2&query=web&hours=6&limit=30
```

### Example 8: Get details of a single alert

```
GET /api/n9e/alert-cur-event/12345
```

---

## Key Considerations

1. **Active alerts vs. historical alerts**: active alerts include only the alerts currently not recovered; historical alerts include both recovered and not-recovered alerts
2. **The severity parameter format differs**: active alerts support comma-separated multiple levels (e.g. `"1,2"`), while historical alerts accept only a single value (e.g. `1`)
3. **No time limit by default**: the active alerts query has no time range limit by default; for historical alerts it is recommended to specify `hours` or `stime`/`etime` to avoid returning too much data
4. **Pagination**: use the `limit` and `p` (page number) parameters for pagination; `limit` defaults to 20
5. **tags format**: the `tags` in the response is a string array in `["key=value"]` format
6. **Time fields are all Unix timestamps (seconds)**: `trigger_time`, `first_trigger_time`, `recover_time`, `last_eval_time`, etc.
7. **query search scope**: the keyword matches both the alert rule name (`rule_name`) and the tags (`tags`)
8. **Alert details URLs differ**: active alerts use `/alert-cur-event/<id>`, historical alerts use `/alert-his-event/<id>` (note the singular/plural)
9. **Output format**: format the results into a Markdown table for the user, including key columns such as alert name, severity, trigger time, target, and trigger value
