---
name: analyze-dashboard
description: Analyze the data health of a given dashboard on Nightingale (n9e) over a time window. Use when the user asks to "analyze what problems a dashboard has", "check whether the xx dashboard has been normal over the last 24 hours", "inspect this dashboard", or "does this dashboard have any anomalies". Distinct from modifying a dashboard (modify-dashboard) and creating a dashboard (create-dashboard).
max_iterations: 12
examples:
  - "Analyze what data problems the etcd dashboard has within 24 hours"
  - "Check whether the Linux host monitoring dashboard has been normal over the last hour"
  - "Inspect this dashboard"
  - "Has the host web01 in this dashboard had any anomalies over the last day"
builtin_tools:
  - list_dashboards
  - get_dashboard_data
  - get_dashboard_detail
  - query_prometheus
  - list_busi_groups
tags:
  - internal
---

# Skill: Nightingale (N9E) Analyze Dashboard

Help the user analyze the data health of an **existing dashboard** over a given time window, and produce an anomaly list with recommendations.

The core tool is `get_dashboard_data`: it performs data fetching and **statistical pre-screening** on the server side (four deterministic detections — MAD outliers, sudden changes, trends, year-over-year comparison — plus periodicity denoising), and returns layered results. Year-over-year baseline: for a window ≤24h it compares against the same period yesterday; for a longer window it compares against the previous period of the same length (the digest header states the shift amount — describe it accordingly). **The detection is already done; your job is attribution, correlation, and explanation, not re-scanning point by point.**

## Step 1: Locate the dashboard

- The context already carries a `dashboard_id` (injected by the frontend from `/dashboards/<id>`) → use it directly.
- The user pasted a `/dashboards/<id>` link → take the id.
- Only a name was given → match by name with `list_dashboards(query="...")`; if there are multiple candidates or no match, list them and ask the user — **do not** guess.

## Step 2: Call get_dashboard_data

```
get_dashboard_data(id=<id>, time_range="24h")
```

- Pass `time_range` according to user intent ("the last day" → `24h`, "this week" → `7d`); if the user did not specify, use the default `1h` and note the analysis window in the conclusion.
- The user constrained host/cluster, etc. ("look at the host web01") → pass it via `vars`: `vars={"ident":["web01"]}` (the variable name follows the variable definition in `get_dashboard_detail(include_config=true)`; if unsure, check it first).
- **One call is enough.** Do not first call get_dashboard_detail and then run query_prometheus over every panel at full scale — that is what the tool has already done for you.

## Step 3: Interpret the pre-screening results (your core value)

The tool returns layered results: ⚠ suspicious curves (features + sampled points) / ✓ normal summary / skipped list (including a **flat-line** category — curves with a constant value are counted separately; if a curve is flat but at a different level than yesterday, it goes into the suspicious section as a year-over-year anomaly, e.g. qps stuck at 0). What you should do:

1. **Cross-panel correlation**: simultaneous movement of multiple curves at the same moment is the most important signal. For example, `14:32 CPU sudden change +312%` co-occurring with `14:32 disk latency outlier` → likely the same event; merge them into a single problem statement rather than listing them as two.
2. **Judge impact in light of metric semantics**: rising WAL fsync latency → writes are affected; a rising connection-count trend approaching the limit → capacity risk. Talk about "what this means", not re-reading the numbers.
3. **Re-examine the "suspected periodic" flag**: spikes flagged as periodic are usually scheduled jobs and generally do not count as anomalies; but if the user is specifically asking "why is it slow at this same time every day", it is in fact the answer.
4. **Drill down when necessary**: for a key suspicious metric, use `query_prometheus` to narrow the time window and inspect details (e.g. ±15 minutes around the sudden change, with a smaller step), or query a related metric to validate a hypothesis. **Drill down into at most 2-3** of the most critical ones; do not query every suspicious curve.
5. **Do not recite the sampled points**: the attached "points:" series is evidence for you to see the shape; in the conclusion just cite the key moments and values.

## Step 4: Output the analysis report

```
## <dashboard name> Health Analysis (window 24h)

**Overall conclusion**: one sentence (e.g.: found 1 suspected disk performance event affecting the etcd-2 node; all other metrics normal)

### Anomalies (by severity)
1. **etcd-2 disk performance degradation (starting at 14:30)**
   - Evidence: WAL fsync latency 4ms→89ms (sudden change +312%@14:30, no such phenomenon in the year-over-year window); CPU spiked to 96% at the same moment
   - Impact: write latency will be driven up directly
2. ...

### Correlation analysis
(correlation inference of multiple metrics over time)

### Recommendations
- Actionable next steps (check the disk of the host where etcd-2 resides / iostat / whether there is a snapshot job...)
```

- When there are no suspicious curves, state clearly "no anomalies found"; **do not fabricate problems**. Note the coverage (window, skipped panels).
- The output was truncated (very large dashboard) → use `panel_ids` to re-call key panels in batches, then merge the conclusions at the end.

## Boundaries and special cases

- The dashboard has no Prometheus panels at all → the tool returns an error and lists the data source distribution; tell the user directly that this is not supported for now.
- When a variable resolves to no value, the tool notes "matched everything with .*"; the data for a multi-instance dashboard may be excessive, so carry this premise in the conclusion.
- A curve that "existed yesterday but is gone today" (an instance disappeared) is worth a mention in the anomalies — losing an instance is often more serious than metric fluctuation. Note that when the window is >24h, this segment actually compares against the previous period of the same length (e.g. the previous 7 days), so do not phrase it as "it was still there yesterday".
- The user then wants to modify the dashboard (add charts / change thresholds) → load modify-dashboard; to create an alert on an anomalous metric → load create-alert-rule.

## Notes

- In the report, always use the moment format returned by the tool (HH:MM); do not convert time zones yourself.
- Keep the precision the tool gives for numbers; the focus is the trend and order of magnitude, not decimal places.
- Suspicious ≠ failure: the detection is statistical, so word the conclusion as "suspected/possible" and make the basis for the judgment clear.
