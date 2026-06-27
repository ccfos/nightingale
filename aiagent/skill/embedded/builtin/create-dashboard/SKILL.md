---
name: create-dashboard
description: Create monitoring dashboards. Use this when the user asks to create a dashboard, a monitoring board, or a Dashboard.
max_iterations: 18
builtin_tools:
  - import_dashboard_template
  - create_dashboard
  - list_metrics
  - list_files
  - read_file
  - grep_files
tags:
  - export
---

# Skill: Nightingale (N9E) Dashboard Creation

There are two paths to create a dashboard. **Prefer path A (import an integration template)**, since templates are hand-tuned, validated, finished products with quality far higher than hand-assembled ones; only fall back to path B when there is no ready-made template or the user wants custom metrics.

When the same component often has both categraf and exporter templates, **prefer categraf**: first probe whether the categraf metrics have data, use the categraf template if you can query them, and only fall back to exporter if you cannot (see path A step 3 for details).

| Scenario | Which tool to use |
|------|-----------|
| The monitoring topic has a ready-made template in integrations (Linux / MySQL / Redis / Kafka / PostgreSQL / Elasticsearch / Ceph / Oracle / Nginx / Windows …) | **`import_dashboard_template`** (preferred) |
| Custom metrics, no ready-made template, or the user explicitly wants custom panels | `create_dashboard` |

## Step 1 (common to both paths): Determine the business group and datasource

- Call `list_busi_groups` to get the list of business groups
- Call `list_datasources` to get the list of datasources, and find the **datasource ID of the Prometheus type**
- If the conversation context has already preselected `busi_group_id` / `datasource_id`, use them directly and do **not** call `list_*` again

### Business group selection rules

1. If the user explicitly specified a business group name or ID, use it directly
2. Otherwise, in priority order:
   a. **Prefer the business group with `is_default: true`** (usually "Default Busi Group" or a group whose name contains "default")
   b. If there is only one business group, use it directly
   c. If there are multiple candidates and none is the default, **do not blindly take the first one**; list them in your reply and let the user confirm

---

## Path A: Import an integration template (preferred)

`import_dashboard_template` reads the complete template under `integrations/` (preserving layout, thresholds, units, overrides, and value mappings in full), and automatically rewrites the template's datasource binding onto the Prometheus datasource you chose.

### Steps

1. See which integration components exist:
   ```
   list_files(base="integrations")
   ```
2. See which template files exist under that component:
   ```
   list_files(base="integrations/Linux", path="dashboards")
   ```
3. **Pick the right template file**. Templates often come in two collection styles, categraf (telegraf-style metric names) and exporter (`node_*` / `*_exporter`-style metric names). File-naming conventions vary per component: it may be a prefix (Linux's `categraf-overview.json`) or a suffix (`redis_by_categraf.json`). **Prefer the template whose file name contains `categraf`**:
   - First probe whether the categraf metrics have data in the environment: `list_metrics(datasource_id=<X>, keyword="<categraf metric keyword>")`.
     - Use a categraf-specific metric name as the keyword that distinguishes the two styles, for example for Linux use `cpu_usage` (categraf has `cpu_usage_idle`, while node_exporter does not have this name), `mem_used_percent`, `disk_used_percent`; for Redis use `redis_used_memory`.
     - If you are unsure which keyword to use, read the metrics list of that template (the file is small and will not be truncated): `read_file(base="integrations/Linux", path="metrics/categraf-base.json")`, and pick a representative `expression` from it to probe with.
   - **As long as the categraf metrics return data (a non-empty array), use the template whose file name contains `categraf` directly**, with no need to compare against exporter.
   - Only when the categraf metrics return no data at all should you fall back to the template whose file name contains `exporter` (such as `exporter-detail.json`, `redis_by_exporter.json`), using the corresponding exporter metric keyword (such as `node_cpu_seconds_total`) with `list_metrics` to confirm it actually has data before importing.
   - **Do not** use `read_file` to read out the entire dashboard template and assemble it yourself — the template may be very large and get truncated; choosing the file by file name + the metrics list is enough
4. Import:
   ```
   import_dashboard_template(group_id=<business group>, component="Linux", file="categraf-overview.json", datasource_id=<Prometheus datasource>)
   ```

### Notes

- Datasource binding, variables, and layout are all handled automatically; you only need to pass `component` + `file`
- `datasource_id` is optional: passing it sets it as the default selected value of the dashboard's datasource variable (so the first screen is queryable immediately); not passing it lets the frontend automatically select the first Prometheus from the datasource dropdown
- To rename or change tags: pass the optional `name` / `tags`; if not passed, the template's own values are kept
- When it returns `Name duplicate`, **do not** call `list_dashboards`; just change the name (add a `-v2`, `-AI`, or timestamp suffix) and retry

---

## Path B: Custom creation (when there is no template)

Use `create_dashboard`. **You only need to provide the panel title, type, and PromQL**; the tool automatically generates the full configuration (layout, datasource variables, styling, units, etc., are all handled automatically).

> create_dashboard accepts the following set of **simplified fields**. Any field beyond these (thresholds, overrides, value mappings, heatmap/hexbin/tableNG/iframe, etc.) is **not supported** — they will be ignored even if written. When you need such rich configuration, switch to path A and import a template.

### Call example

```json
{
  "group_id": 1,
  "name": "Linux Host Monitoring",
  "datasource_id": 1,
  "tags": "linux host",
  "variables": "[{\"name\":\"ident\",\"label\":\"Host\",\"definition\":\"label_values(cpu_usage_idle, ident)\"}]",
  "panels": "[{\"name\":\"CPU Usage\",\"type\":\"stat\",\"queries\":[{\"promql\":\"avg(cpu_usage_active{cpu=\\\"cpu-total\\\",ident=~\\\"$ident\\\"})\",\"legend\":\"CPU\"}],\"unit\":\"percent\"},{\"name\":\"CPU Usage Trend\",\"type\":\"timeseries\",\"queries\":[{\"promql\":\"cpu_usage_active{cpu=\\\"cpu-total\\\",ident=~\\\"$ident\\\"}\",\"legend\":\"{{ident}}\"}],\"unit\":\"percent\"}]"
}
```

### Panel fields (only these are supported)

Each panel requires 3 fields:

```json
{"name": "Panel title", "type": "timeseries", "queries": [{"promql": "PromQL expression", "legend": "{{ident}}"}]}
```

| Field | Description | Default |
|------|------|------|
| `name` | Panel title (required) | — |
| `type` | Panel type (required, see table below) | — |
| `queries` | Query list, each item `{promql, legend?, instant?}` | — |
| `unit` | Unit | none |
| `w` / `h` | Width/height (number of grid columns, total width 24) | auto by type |
| `stack` | Whether to stack (timeseries only) | false |
| `description` | Panel description | none |

Query field `instant`: for single-value panels such as stat/gauge/barGauge/pie/table, it is recommended to set `"instant": true` (instant query).

### Supported panel types (only these 8)

| type | Description | Default size w×h |
|------|------|--------------|
| `timeseries` | Time-series line chart, the most common | 12×8 |
| `stat` | Single-value statistic big number | 6×4 |
| `gauge` | Gauge | 6×6 |
| `barGauge` | Horizontal bar ranking | 8×8 |
| `pie` | Pie chart | 6×6 |
| `table` | Table | 12×10 |
| `text` | Text note (uses description as content) | 6×4 |
| `row` | Grouping row (automatically full width) | 24×1 |

### Common units (unit)

`percent` | `bytesIEC` | `bitsIEC` | `bytesSecIEC` | `bitsSecIEC` | `seconds` | `milliseconds` | `reqps`

### Automatic layout calculation

Panels are arranged automatically from left to right and top to bottom, with same-type panels auto-aligned (e.g. 4 stat panels in one row); a `row` takes a whole row on its own as a grouping title. No need to specify coordinates manually.

### Variables

```json
{"name": "ident", "label": "Host", "definition": "label_values(cpu_usage_idle, ident)"}
```

`label` and `multi` are optional (multi defaults to true). A later variable references an earlier variable in its definition to achieve cascading:

```json
[
  {"name": "ident", "definition": "label_values(cpu_usage_idle, ident)"},
  {"name": "interface", "definition": "label_values(net_bytes_recv{ident=~\"$ident\"}, interface)"}
]
```

### Notes

- For multi-select variables, use `=~` rather than `=` in PromQL, e.g. `ident=~"$ident"`
- For counter-type metrics, use `rate(...[3m])` or `irate(...[5m])`
- **Prefer copying validated PromQL expressions from the integrations templates**: `read_file(base="integrations/Linux", path="dashboards/categraf-detail.json")`, focusing on the `expr` inside targets
- When it returns `Name duplicate`, just change the name and retry; do not call `list_dashboards`

---

## Step 3 (common to both paths): Output the result

**Keep it short**. A one-sentence confirmation is enough, for example:

> ✅ I have created the dashboard "Linux Host Monitoring" for you; see the card below for details.

**Do not** restate fields such as the dashboard ID, business group, datasource, or panel list — the frontend will display them in a structured card. You may add a sentence or two of additional suggestions, but do not enumerate the fields the card already has.

## Recommended panel design when there is no template (path B reference)

> These topics basically all have templates in integrations; prefer path A. The following is only for reference when hand-building via path B.

### Linux host monitoring

**Variables:** `ident` (host), `interface` (network interface), `mountpoint` (mount point)

| Area | Panel | Type | Core metric |
|------|------|------|----------|
| Overview | CPU Usage | stat | `avg(cpu_usage_active{cpu="cpu-total"})` |
| Overview | Memory Usage | stat | `avg(mem_used_percent)` |
| Overview | Disk Usage (max) | stat | `max(disk_used_percent)` |
| CPU | CPU Usage Trend | timeseries | `cpu_usage_active` + `cpu_usage_iowait` |
| CPU | System Load | timeseries | `system_load1/5/15` |
| Memory | Memory Usage Trend | timeseries | `mem_used_percent` |
| Disk | Per-mount-point Usage | barGauge | `disk_used_percent` |
| Network | Network Traffic | timeseries | `rate(net_bytes_recv/sent)` |

### MySQL monitoring
QPS/TPS, connection count, slow queries, Buffer Pool hit ratio, replication lag

### Redis monitoring
OPS, memory usage, connection count, hit ratio, keyspace

### Kubernetes monitoring
**Variables:** `cluster`, `namespace`, `pod`. Pod CPU/memory, node resources, deployment status, PV usage
