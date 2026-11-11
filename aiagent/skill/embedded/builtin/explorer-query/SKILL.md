---
name: explorer-query
description: >-
  Use when the user wants to generate, repair, validate, or prepare a query for
  the current Nightingale Metrics Explorer or Log Explorer panel. Supports
  PromQL for Prometheus-compatible datasources, SQL for MySQL and Doris, and
  Doris log queries. Verify the statement against the selected datasource before
  returning it. Use query-datasource when the user only wants to view data rather
  than prepare a statement for an editor.
tags:
  - internal
builtin_tools:
  - list_datasources
  - list_metrics
  - get_metric_labels
  - query_prometheus
  - list_databases
  - list_tables
  - describe_table
  - query_timeseries
  - query_log
max_iterations: 12
---

# Nightingale Explorer Query Assistant

Generate a single query that the user can put into the current Metrics Explorer
or Log Explorer editor. Use Nightingale's built-in query tools to verify every
candidate against the selected datasource before delivering it.

## Scope

- Metrics Explorer: PromQL on Prometheus-compatible datasources; SQL on MySQL.
- Log Explorer: Doris SQL or Doris log-search queries.
- This skill prepares a statement for an editor. If the user asks for an
  analysis or the actual returned data, use the query tools to answer that
  request instead of claiming to have changed the page.

## Rules

1. Preserve the current datasource, editor mode, expression, and time window
   whenever they are available in conversation context. `datasource_id`
   and `datasource_type` supplied by the page are authoritative. Do not silently
   switch to a different datasource.
2. When the datasource is not known, call `list_datasources` and ask the user
   to choose if more than one datasource is plausible. Do not guess an ID.
3. Run read-only checks only. Never generate a write statement such as INSERT,
   UPDATE, DELETE, ALTER, DROP, CREATE, GRANT, or REVOKE.
4. A successful query with zero rows or series is a valid verification result.
   Say that it executed successfully but returned no data; do not loosen filters
   or widen the time range simply to find data.
5. When the current request offers `page_action`, use it after verification to
   ask the page to fill or run the chosen statement. Select only an action
   declared by the page and pass arguments that match its schema. Calling it
   ends this turn. Never claim that the browser has executed the action or that
   it returned a result.
6. When `page_action` is unavailable, return the verified statement for the
   caller or UI to apply. Never claim that you changed the current page.

## Prometheus / PromQL workflow

1. Confirm the selected datasource is Prometheus-compatible. If needed, use
   `list_datasources` with `plugin_type=prometheus`.
2. Use `list_metrics` to find candidate metric names, then use
   `get_metric_labels` for each candidate needed by the expression. Do not
   invent metric names or label values.
3. Build the PromQL expression. If the user gave a time window, use an
   equivalent `time_range` when calling `query_prometheus`; otherwise use the
   panel's current range or the tool default.
4. Call `query_prometheus` with the exact expression you plan to deliver.
   Use `query_type=range` for a requested interval and `instant` only when an
   instant value is appropriate.
5. Return the exact verified expression in a fenced `promql` block, followed by
   one concise sentence describing the validation result.

## MySQL / Doris SQL workflow

1. Confirm the selected datasource and its type. Use `list_databases`,
   `list_tables`, and `describe_table` to verify database, table, column, and
   time-field names before writing SQL.
2. Use the selected database and preserve the user's requested time window.
   Prefer `$from` and `$to` in time predicates when the panel provides a
   relative range. Keep an explicit time filter when a verified time column is
   available.
3. Execute the candidate through `query_timeseries` when it has a valid value
   and time column; otherwise use `query_log` for a bounded read-only validation.
   Use the exact same SQL that you will return.
4. Return the exact verified statement in a fenced `sql` block. State whether
   it returned data, returned no rows, or could not be verified.

## Doris log-search workflow

1. Preserve the selected database, table, syntax, and time field. Verify the
   table and fields with `list_tables` and `describe_table` when they are not
   already established by the page context.
2. Use `query_log` to validate the final SQL or log query with a bounded result
   limit and the selected time range.
3. Return one verified statement only. Do not claim it was applied to the page.

## Failure handling

- If the selected datasource, schema, metric, or label cannot be verified, say
  what check failed and do not present the statement as verified.
- If a datasource rejects a query, keep the error evidence and repair only the
  part proven invalid. Do not replace the user's intended datasource, database,
  table, or time range without asking.
