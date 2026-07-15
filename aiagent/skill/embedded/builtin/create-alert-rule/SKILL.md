---
name: create-alert-rule
description: |
  **Create alert rules**. Prefer reusing the validated rules in integrations (standard components like Linux/MySQL/Redis/Kafka/PostgreSQL/Elasticsearch all ship ready-made rule packs); import as many rules as the user needs—one rule, a batch, or a whole pack. Only hand-write a custom rule when integrations has nothing that fits. Supports all data sources: Prometheus / Loki / ES / OpenSearch / MySQL / PG / TDengine / ClickHouse / Doris / VictoriaLogs / Host.
  ⚠️ **Do NOT use this skill for bulk YAML imports**—when the user provides a URL or a YAML file, awesome-prometheus-alerts, node-exporter.yml, and the like, use import-prom-rule instead.
  Triggers: create an alert / add an alert / help me set up a CPU alert / add a set of alert rules for MySQL / configure common alerts for a host / I want to monitor a metric.
examples:
  - "Configure a set of common alert rules for the host"
  - "Add a set of alert rules for MySQL"
  - "Help me create an alert for CPU usage exceeding 80%"
  - "Add a MySQL slow query alert rule"
  - "Add a memory alert for the host, fire when it exceeds 90%"
max_iterations: 20
builtin_tools:
  - preview_alert_rule_template
  - import_alert_rule_template
  - create_alert_rule
  - list_busi_groups
  - list_datasources
  - list_metrics
  - list_notify_rules
  - list_files
  - read_file
  - list_databases
  - list_tables
  - describe_table
tags:
  - export
---

# Skill: Nightingale (N9E) Alert Rule Creation

Two tools: `import_alert_rule_template` (reuse the validated rules in integrations) and `create_alert_rule` (hand-write rules).

**Prefer reusing the rules in integrations**: the rules under `integrations/<component>/alerts/` are finished products that have been hand-tuned and validated (severity, duration, evaluation interval, appended tags, annotations, and effective time windows are all configured), and their quality far exceeds hand-crafted rules. **But import as many as the user needs—do not blindly import the whole pack**:

- The user wants "a set / common" alerts for some component → import the whole pack (do not pass `names`)
- The user names only a few categories (e.g. "disk and memory alerts") → import only those (pass them in `names`)
- The user wants a **single** alert for a specific metric → if the pack happens to contain a matching rule, import just that one (pass one entry in `names`) or borrow its expression; if the pack has nothing that fits, hand-write with `create_alert_rule`

Only go to Approach B (`create_alert_rule`) when integrations has no matching rule, or when the user provides a fully custom query condition.

When a component ships both a categraf and an exporter rule pack, **prefer categraf**: first probe whether the categraf metrics have data; if found, use the categraf pack, otherwise fall back to the exporter pack (see Approach A step 3 for details).

| User need | What to do |
|------|-----------|
| Configure a set / a few alerts for a standard component (Linux / MySQL / Redis / Kafka / PostgreSQL / Elasticsearch / Ceph / Oracle / Windows / MongoDB …) | Approach A: `preview_alert_rule_template` to see what's in the pack → `import_alert_rule_template` to import as needed |
| A single alert for a specific metric, and the integration pack has a matching rule | Approach A: `import_alert_rule_template` + `names` to import just that one |
| Fully custom rules that integrations doesn't cover (including niche conditions like Loki / ES / SQL query values / log counts) | Approach B: `create_alert_rule` |

> ⚠️ **Do NOT use this skill for bulk YAML imports**—when the user provides a URL or a YAML file, awesome-prometheus-alerts, node-exporter.yml, and the like, use import-prom-rule instead.

## Step 1 (common to both paths): determine the business group and data source

- Call `list_busi_groups` to get the list of business groups
- Call `list_datasources` to get the list of data sources, and find the data source ID of the matching type (rule packs are mostly the Prometheus type)
- If the session context already has a preselected `busi_group_id` / `datasource_id`, use it directly—**do not** call `list_*` again
- `cate=host` (host-unreachable class) does not need a data source

### Business group selection rules

1. If the user explicitly specified a business group name or ID, use it directly
2. Otherwise, by priority:
   a. **Prefer the business group with `is_default: true`** (usually "Default Busi Group" or a group whose name contains "Default")
   b. If there is only one business group, use it directly
   c. When there are multiple candidates and none is the default, **do not blindly take the first one**; list them in your reply and ask the user to confirm

---

## Approach A: reuse validated rules in integrations (as needed: single / batch / whole pack)

`import_alert_rule_template` imports rules from the rule pack under `integrations/<component>/alerts/` (each rule's severity, duration, evaluation interval, appended tags, annotations, and effective time window are all preserved), binds the rules to the data source you select, and uniformly switches them to the enabled state. **How many rules are imported is controlled by the `names` parameter**, driven by the user's need.

### Steps

1. See which integration components are available:
   ```
   list_files(base="integrations")
   ```
2. See which rule pack files exist under that component:
   ```
   list_files(base="integrations/Linux", path="alerts")
   ```
3. **Pick the right rule pack file (prefer categraf)**. Rule packs often come in two collection styles, categraf and exporter, and the filename convention varies per component (for Linux it's `linux_by_categraf.json` / `linux_by_exporter.json`, but it may use other suffixes). **Prefer the pack whose filename contains `categraf`**:
   - First probe whether the categraf metrics have data in the environment: `list_metrics(datasource_id=<X>, keyword="<categraf metric keyword>")`.
     - Use a categraf-specific metric name that distinguishes the two styles as the keyword, e.g. for Linux use `cpu_usage` (categraf uses `cpu_usage_idle`, which node_exporter does not have), `mem_used_percent`, `disk_used_percent`; for Redis use `redis_used_memory`; for MySQL use `mysql_global_status_queries`.
   - **As long as the categraf metric returns data (a non-empty array), use the pack whose filename contains `categraf`** directly, without comparing against exporter.
   - Only fall back to the pack whose filename contains `exporter` (e.g. `linux_by_exporter.json`) when the categraf metric has no data, and use the corresponding exporter metric keyword (e.g. `node_cpu_seconds_total`) with `list_metrics` to confirm data exists before importing.
4. **See which rules are in the pack (only when you need to select by name)**. You can skip this step when the user wants to import the whole pack; when the user wants only a few of them, or a single alert for a specific metric, preview first to get the exact rule names:
   ```
   preview_alert_rule_template(component="Linux", file="linux_by_categraf.json")
   ```
   It returns each rule's `name` / `cate` / `severity` / an expression summary / whether it is disabled (a small payload). **Do not** use `read_file` to view a large rule pack (it will be truncated); use this tool.
5. **Import as needed**:
   - Whole pack (the user wants "a set / common" alerts)—do not pass `names`:
     ```
     import_alert_rule_template(group_id=<business group>, component="Linux", file="linux_by_categraf.json", datasource_id=<data source>)
     ```
   - Batch / single (the user names a few categories or a specific metric)—`names` carries the exact rule names (from preview):
     ```
     import_alert_rule_template(group_id=<business group>, component="Linux", file="linux_by_categraf.json",
       datasource_id=<data source>, names="[\"Machine load - high memory, please pay attention - categraf\"]")
     ```

> **Do not** use `read_file` to read out the entire rule pack and then call `create_alert_rule` one rule at a time—the pack may be large and get truncated, and converting rules one by one also drops fields. To see what's in the pack use `preview_alert_rule_template`; to persist it use `import_alert_rule_template`.

### Notes

- **Strongly recommend passing `datasource_id`**: passing it binds the imported rules to that data source; omitting it makes the rules match every data source of that type (an overly broad scope). The `host` heartbeat rules in the pack don't need a data source, and the tool skips binding for them automatically.
- The rules in a rule pack are **disabled** by default in the template (it is a browsable catalog). When importing, this tool **switches them to enabled** by default (`disabled=0`) so the rules take effect immediately. If the user wants to "import first but leave them disabled, and enable them one by one themselves", pass `disabled=1`.
- The names written in `names` must exactly match the `name` returned by `preview_alert_rule_template`; any that don't match are listed in the returned `not_found_names`—double-check and retry.
- Duplicate rules (already existing in the business group) are **skipped automatically** (`status=skipped_duplicate`); this is not a failure, and **do not** re-import with `name_prefix` just because you see a skip. Only use `name_prefix`/`name_suffix` when the user explicitly wants the rule to coexist with the existing one (e.g. a comparison test).
- The return includes three counts—`created` / `skipped` / `failed`—plus details for each rule; just summarize them in the output.
- Imported rules **are not bound to any notify rule** by default (to avoid sending alerts by mistake). If binding is needed, after importing, prompt the user to associate it manually on the rule, or switch to Approach B to create rules one by one and pass `notify_rule_ids`.

---

## Approach B: hand-write a custom rule (when integrations has no matching rule)

Use `create_alert_rule`. Supports all data source types: **Prometheus / Loki / Elasticsearch / OpenSearch / TDengine / ClickHouse / MySQL / PostgreSQL / Doris / VictoriaLogs / Host**.

> Before getting here, confirm that integrations really has no matching rule: common metrics for standard components (CPU/memory/disk/connection count…) are mostly in the rule packs, so reuse rather than hand-craft when you can. Even when hand-crafting, you can first run `preview_alert_rule_template` to glance at whether there's a ready-made expression you can borrow.

The tool offers two invocation modes:

1. **Prometheus simplified path** (most common)—pass `prom_ql` + `threshold` + `operator` directly, and the tool builds the rule automatically
2. **Generic path**—pass `cate` + `rule_config_json`; **first obtain the structure of `rule_config_json` by reading `datasources/<cate>.md` via `read_file`**, then fill in the actual values

### Mode 1: Prometheus simplified path

The most common scenario. You only need to fill in PromQL, threshold, and operator. Internally the tool splices the threshold into `prom_ql` to generate a v1-format rule (the v2 editor in the OSS n9e FE is gated by `IS_PLUS`, so only v1 can be used).

```json
{
  "group_id": 1,
  "name": "CPU usage too high",
  "datasource_id": 1,
  "prom_ql": "avg by (ident) (100 - cpu_usage_idle{cpu=\"cpu-total\"})",
  "operator": ">",
  "threshold": 80,
  "severity": 2,
  "note": "CPU usage exceeded 80% for 1 minute",
  "for_duration": 60
}
```

### Mode 2: generic path (non-Prometheus)

For types such as Loki / ES / MySQL / TDengine / ClickHouse / Doris / VictoriaLogs / Host, you need to pass `cate` and `rule_config_json`.

**Key step**: first read the reference doc for that data source type to obtain the `rule_config` structure template:

```
read_file(base="create-alert-rule", path="datasources/<cate>.md")
```

Then turn the `rule_config` object in the doc into a JSON string and pass it to the `rule_config_json` parameter. Example (MySQL alert):

```json
{
  "group_id": 1,
  "name": "MySQL too many failed orders",
  "cate": "mysql",
  "datasource_id": 4,
  "severity": 2,
  "rule_config_json": "{\"queries\":[{\"ref\":\"A\",\"sql\":\"SELECT count(*) AS value FROM orders WHERE created_at >= NOW() - INTERVAL 5 MINUTE AND status='failed'\",\"keys\":{\"valueKey\":\"value\",\"labelKey\":\"\"},\"interval\":60}],\"triggers\":[{\"mode\":1,\"exp\":\"$A.value > 10\",\"severity\":2,\"recover_config\":{\"judge_type\":1}}]}"
}
```

**⚠️ Important generic rules (apply to all non-Prometheus types)**:

1. **The `interval` field must be the total number of seconds**—do not write `interval_unit`. When the frontend saves, it converts `value × unit → seconds`, and when reading it derives the display unit back from the seconds.
   - Last 1 minute → `"interval": 60`
   - Last 5 minutes → `"interval": 300`
   - Last 1 hour → `"interval": 3600`
   - **Do not** write `"interval": 5, "interval_unit": "min"`—the FE will display it as 5 seconds.
   - The tool has a defensive fallback: if you accidentally write `interval_unit` or a bare interval smaller than 60, it converts it to seconds automatically, but it's best to write it correctly to begin with.

**⚠️ Important limitations for OSS n9e SQL-type data sources (MySQL/PGSQL/CK/Doris)**:
1. **`keys.valueKey` is required**—the alias of the numeric column in the SELECT statement, usually `"value"`. Missing it reports `valueKey is required`.
2. **`$from`/`$to`/`$__timeFilter` are not substituted**—the OSS `macros.Macro` is a no-op. You must use the data source's native time functions, e.g. MySQL uses `NOW() - INTERVAL 5 MINUTE`, PG uses `NOW() - INTERVAL '5 minutes'`, CK uses `now() - INTERVAL 5 MINUTE`.
3. **TDengine is the exception**—it has its own variable substitution logic, and `$from`/`$to`/`$interval` are usable.

## create_alert_rule parameter reference (Approach B)

| Field | Required | Description |
|---|---|---|
| `group_id` | ✅ | Business group ID (from `list_busi_groups`) |
| `name` | ✅ | Rule name (must be unique within the business group) |
| `cate` | ❌ | Data source type (default `prometheus`). Options: `prometheus` / `loki` / `elasticsearch` / `opensearch` / `tdengine` / `ck` / `mysql` / `pgsql` / `doris` / `victorialogs` / `host` |
| `prod` | ❌ | Product type. When omitted, derived automatically from `cate` |
| `datasource_id` | Conditionally required | Data source ID. **Not needed when `cate=host`**; required for all others |
| `rule_config_json` | Conditionally required | The complete `rule_config` JSON string. **Can be omitted when `cate=prometheus` (use the simplified path)**; required for other types |
| `prom_ql` | Conditionally required | PromQL query (only for the `cate=prometheus` simplified path). **Write only the query, not the threshold** |
| `threshold` | Conditionally required | Trigger threshold (only for the simplified path) |
| `operator` | ❌ | Default `>`. Options `>` `>=` `<` `<=` `==` `!=` |
| `severity` | ❌ | Default 2. 1=Critical, 2=Warning, 3=Info |
| `note` | ❌ | Alert notification body |
| `eval_interval` | ❌ | Evaluation interval (seconds), default 30 |
| `for_duration` | ❌ | Duration (seconds), default 60 |
| `append_tags` | ❌ | Appended tags, multiple separated by spaces |
| `runbook_url` | ❌ | Runbook URL |
| `notify_rule_ids` | ❌ | JSON list of associated notify rule IDs, e.g. `"[1,2]"` |

## Approach B detailed steps (create_alert_rule)

> See "Step 1 (common to both paths)" above for selecting the business group and data source. Below are the steps specific to Approach B.

### B-1: identify the data source type (cate)

Based on the user's alert need, decide which `cate` to use:

| User-need keyword | cate | Trigger condition |
|---|---|---|
| "Host CPU/memory/disk", "Prometheus metric" | `prometheus` | Metric threshold |
| "Machine unreachable", "node offline" | `host` | Heartbeat timeout |
| "Application error log", "Loki log" | `loki` | Log count |
| "ES log", "Elasticsearch aggregation" | `elasticsearch` | Log aggregation |
| "OpenSearch log" | `opensearch` | Log aggregation (same as ES) |
| "MySQL abnormal query result" | `mysql` | SQL query value |
| "PostgreSQL abnormal query result" | `pgsql` | SQL query value |
| "ClickHouse metric/log" | `ck` | SQL query value |
| "Doris log" | `doris` | SQL query value |
| "TDengine time-series data" | `tdengine` | SQL query value |
| "VictoriaLogs log" | `victorialogs` | LogsQL query |

Default value: `prometheus`. The data source must match too (`cate=mysql` finds a MySQL data source, `cate=loki` finds a Loki data source, etc.); `cate=host` does not need a data source.

### B-2 (non-Prometheus only): read the data source reference doc

For types where `cate != "prometheus"`, **you must first read the reference doc for that type** to obtain the `rule_config` structure:

```
read_file(base="create-alert-rule", path="datasources/<cate>.md")
```

For example:
- MySQL → `datasources/mysql.md`
- Loki → `datasources/loki.md`
- Host → `datasources/host.md`

The reference docs contain complete field descriptions, option-value tables, and complete examples. **Copy the field names from the `rule_config` object in the example as-is**, replacing only the concrete values.

### B-3 (SQL types only): probe the real schema ⚠️

**This step is mandatory when `cate ∈ {mysql, pgsql, ck, doris, tdengine}`; it cannot be skipped**.

> **TDengine is also supported**: although TDengine is a time-series database, its REST API supports `SHOW DATABASES` / `information_schema.ins_tables` / `information_schema.ins_columns`. `list_databases` / `list_tables` / `describe_table` are fully usable with TDengine.

**Never guess database/table/column names from the literal description in the user's prompt**. When the user says "query the access_log table in the my_logs database",
it is very likely just an example, and the actual data source may have no such database or table at all.

You must probe in order:

1. **`list_databases(datasource_id=<X>)`**—see the real list of databases
   - Compare against the user's description and find the closest one
   - If the database name in the user's description **is not in the returned list**, **do not force it into the SQL**. Use AskUserQuestion
     to list the real database names for the user to confirm, or pick one that looks relevant and have the user approve it

2. **`list_tables(datasource_id=<X>, database=<the chosen database>)`**—see the real list of tables
   - Same as above: if the table the user mentioned is not in the list → list the real tables for the user to choose

3. **`describe_table(datasource_id=<X>, database=<database>, table=<table>)`**—get the real columns
   - Use the returned column names to assemble the SQL—**do not invent** common column names like `log_time`, `status`, `created_at`
     (they may not actually exist in this table)
   - Pay special attention to the time column: in different tables it may be called `ts`, `event_time`, `log_time`, `create_time`,
     `timestamp`, etc.; you must decide based on the column names returned by `describe_table`

Only after obtaining the real schema can you proceed to B-4 and assemble the SQL.

#### How to pass datasource_id

- When the session context already binds a data source (a conversation entered from the data source page): `datasource_id` can be omitted,
  inherited from the session context
- In the alert rule scenario (entered from the alert rule page or a plain conversation): you must **explicitly pass** `datasource_id`,
  otherwise the tool reports a `datasource_id required` error

### B-4: build the PromQL / SQL / LogQL

#### Simplified path (cate=prometheus)

Based on the user's description, build a PromQL **without a threshold**. All entries in the table below are **categraf (telegraf-style)** metric names—**prefer categraf**:

| User need | Recommended PromQL (prefer categraf) |
|---|---|
| Host CPU usage | `avg by (ident) (100 - cpu_usage_idle{cpu="cpu-total"})` |
| Host memory usage | `mem_used_percent` |
| Host disk usage | `disk_used_percent` |
| Host system load | `system_load1` |
| Host inbound network traffic | `rate(net_bytes_recv[5m])` |
| MySQL QPS | `rate(mysql_global_status_queries[5m])` |
| MySQL connection usage | `mysql_global_status_threads_connected / mysql_global_variables_max_connections * 100` |
| Redis memory usage | `redis_used_memory / redis_maxmemory * 100` |

**Prefer categraf + probe**: first use `list_metrics` to confirm whether the categraf metric has data in the environment, e.g.
`list_metrics(datasource_id=<X>, keyword="cpu_usage")` (categraf uses `cpu_usage_idle`, which node_exporter does not have).
- **Data found** → use the categraf PromQL from the table above directly.
- **Not found** → the environment very likely uses node_exporter; switch to the equivalent exporter expression below, and use
  `list_metrics(datasource_id=<X>, keyword="node_")` again to confirm the node_exporter metrics actually have data before creating the rule.

##### node_exporter fallback equivalence table (use only when the categraf metric has no data)

| User need | node_exporter equivalent PromQL |
|---|---|
| Host CPU usage | `100 - avg by (instance) (rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100` |
| Host memory usage | `(1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) * 100` |
| Host disk usage | `(1 - node_filesystem_avail_bytes{fstype!~"tmpfs\|overlay"} / node_filesystem_size_bytes) * 100` |
| Host system load | `node_load1` |
| Host inbound network traffic | `rate(node_network_receive_bytes_total[5m])` |

> The `\|` in the disk expression is only escaped to avoid breaking the Markdown table; when writing it into PromQL, restore it to `|`, i.e. `fstype!~"tmpfs|overlay"` (otherwise the regex treats it as a literal pipe and the filter fails).

#### Generic path (other cate)

Following the reference doc read in B-2, assemble the `rule_config` object. Key fields:

- **SQL types** (mysql/pgsql/ck/doris):
  - The query must return one column aliased `value` as the alert judgment value
  - **Required** `keys.valueKey: "value"` (missing → `valueKey is required` error)
  - **Time filtering cannot use `$from`/`$to`**; use the data source's native time functions:
    - MySQL: `NOW() - INTERVAL 5 MINUTE`
    - PostgreSQL: `NOW() - INTERVAL '5 minutes'`
    - ClickHouse: `now() - INTERVAL 5 MINUTE`
    - Doris: `NOW() - INTERVAL 5 MINUTE`
- **TDengine**: a special exception; `$from`/`$to`/`$interval` **can be used**, and use `keys.metricKey` instead of `keys.valueKey`
- **Log types** (loki/es/opensearch/victorialogs): the aggregation query returns a numeric value; use `0` for `recover_config.judge_type`
- **Metric types** (prometheus/mysql/pgsql/ck/tdengine): use `1` for `recover_config.judge_type`
- **Host type** (host): `queries` is a `{key, op, values}` structure; `triggers` is a `{type, severity, duration}` structure; no `datasource_id` needed

### B-5: call create_alert_rule

#### Simplified path (Prometheus)

```
create_alert_rule(
  group_id=1, name="CPU too high", datasource_id=1,
  prom_ql="avg by (ident) (100 - cpu_usage_idle{cpu=\"cpu-total\"})",
  threshold=80, operator=">", severity=2
)
```

#### Generic path (other types)

```
create_alert_rule(
  group_id=1, name="MySQL too many errors",
  cate="mysql", datasource_id=4, severity=2,
  rule_config_json="<serialize the rule_config read in step 3 into a JSON string>"
)
```

**Notes:**
- On the Prometheus simplified path, pass the threshold separately in `threshold`—**do not write it into `prom_ql`**
- Default `for_duration=60`, which is only reasonable if it is larger than `eval_interval=30`
- If the user requests "fire immediately", set `for_duration` to 0
- If `create_alert_rule` returns an `already exists` error, **do not call `list_alert_rules`** to look up the duplicate name; just add a suffix to the name (e.g. `-v2`, `-AI`, or a timestamp) and retry
- On the generic path, `rule_config_json` must be a **JSON string** (not an object); remember to escape the quotes
- **For SQL types, the B-3 schema probe must have been completed before the call**. If the database/table/column in rule_config_json has not been verified via `list_databases`+`list_tables`+`describe_table`,
  the alert rule will most likely fail to run (`Unknown database` / `Unknown column` and other runtime errors)

### B-6: (optional) associate notify rules

If the user requests binding notify rules:
1. Call `list_notify_rules` to get the list of notify rules
2. When calling `create_alert_rule`, pass `notify_rule_ids`, e.g. `"[1,2]"`

If the user does not explicitly request it, **do not proactively associate notify rules**, to avoid sending alerts by mistake.

---

## Final step (common to both paths): output the result

**Keep it short**. A one-sentence confirmation is enough:

- Approach A (importing a rule pack): summarize the three counts `created` / `skipped` / `failed`, e.g.
  > ✅ Imported 8 alert rules from the Linux categraf rule pack for you (skipped 1 duplicate name); all are enabled, see the card below for details.
- Approach B (single creation):
  > ✅ Created the alert rule "Host CPU usage too high" for you; see the card below for details.

**Do not** restate fields such as rule ID, business group, data source, PromQL, threshold, or alert severity in the Final Answer—the frontend displays this information as a structured card, and repeating it only makes the user see two copies. If you need to add a note (e.g. "these rules are not bound to any notify rule by default; please associate them as needed"), you may add a sentence or two, but do not list out the fields already in the card one by one.

## Severity quick reference

| severity | Name | Applicable scenario |
|---|---|---|
| 1 | Critical (level 1) | Service unavailable, core component down, data loss |
| 2 | Warning (level 2) | High resource usage, performance degradation, quota approaching limit |
| 3 | Info (level 3) | Informational alert, capacity planning hint |

Use 2 (Warning) by default. Use 1 only when the user explicitly says "severe" or "urgent".

## Common single-alert templates (Approach B reference)

> Use these when the user wants a single alert for a specific metric. If the user wants to "add a whole set of monitoring alerts for a host",
> prefer Approach A to import the whole `integrations/Linux/alerts/linux_by_categraf.json` pack, rather than hand-crafting rules one by one here.

### Host CPU usage alert
```json
{
  "group_id": 1,
  "name": "Host CPU usage too high",
  "datasource_id": 1,
  "prom_ql": "avg by (ident) (100 - cpu_usage_idle{cpu=\"cpu-total\"})",
  "operator": ">",
  "threshold": 80,
  "severity": 2,
  "for_duration": 300,
  "note": "Host {{ $labels.ident }} CPU usage exceeded 80% for 5 minutes"
}
```

### Host memory usage alert
```json
{
  "group_id": 1,
  "name": "Host memory usage too high",
  "datasource_id": 1,
  "prom_ql": "mem_used_percent",
  "operator": ">",
  "threshold": 90,
  "severity": 1,
  "for_duration": 180
}
```

### Host disk usage alert
```json
{
  "group_id": 1,
  "name": "Host disk usage too high",
  "datasource_id": 1,
  "prom_ql": "disk_used_percent",
  "operator": ">",
  "threshold": 85,
  "severity": 2,
  "for_duration": 600,
  "append_tags": "alert_type=capacity"
}
```
