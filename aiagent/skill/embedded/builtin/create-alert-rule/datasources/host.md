# Host machine monitoring alert rules

- `prod`: `"host"`
- `cate`: `"host"`
- **No need to specify datasource_ids**

The Host type is rather special; its queries and triggers structures are completely different from the other types.

## rule_config structure

```json
{
  "rule_config": {
    "queries": [
      {
        "key": "all_hosts",
        "op": "==",
        "values": []
      }
    ],
    "triggers": [
      {
        "type": "target_miss",
        "severity": 2,
        "duration": 30
      }
    ]
  }
}
```

## queries field reference

### key options

| key | Description | values example |
|---|---|---|
| `all_hosts` | All hosts | `[]` |
| `group_ids` | Filter by business group | `[1, 2, 3]` |
| `tags` | Filter by tag | `["env=prod", "region=cn"]` |
| `hosts` | Filter by hostname | `["web-01", "web-02"]` |

### op options

| op | Description |
|---|---|
| `==` | Equals |
| `!=` | Not equals |
| `=~` | Regex match (only supported for the `hosts` key) |
| `!~` | Regex non-match (only supported for the `hosts` key) |

Multiple queries are in an AND logical relationship.

## triggers field reference

### type options

| type | Description | Extra field |
|---|---|---|
| `target_miss` | Machine unreachable | `duration` (seconds) |
| `pct_target_miss` | Percentage of machines unreachable | `duration` (seconds) + `percent` (percentage) |
| `offset` | Time offset too large | `duration` (seconds) |

## Complete example (machine unreachable alert)

```json
[{
  "name": "Machine unreachable alert",
  "note": "Machine has not reported data for more than 60 seconds",
  "prod": "host",
  "cate": "host",
  "datasource_ids": [],
  "datasource_queries": [{"match_type": 0, "op": "in", "values": []}],
  "disabled": 0,
  "prom_eval_interval": 30,
  "prom_for_duration": 0,
  "rule_config": {
    "queries": [{"key": "all_hosts", "op": "==", "values": []}],
    "triggers": [{"type": "target_miss", "severity": 1, "duration": 60}]
  },
  "enable_in_bg": 0,
  "enable_days_of_weeks": [["0","1","2","3","4","5","6"]],
  "enable_stimes": ["00:00"],
  "enable_etimes": ["00:00"],
  "notify_recovered": 1,
  "notify_repeat_step": 60,
  "notify_max_number": 0,
  "callbacks": [],
  "append_tags": [],
  "annotations": {},
  "extra_config": {},
  "notify_version": 1,
  "notify_rule_ids": []
}]
```
