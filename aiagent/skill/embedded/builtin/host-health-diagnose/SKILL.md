---
name: host-health-diagnose
description: Help the user determine whether a machine is truly down / the agent is hung / the network is flapping / it is under maintenance. Triggers this skill when the user asks "why is this machine unreachable", "is the host-unreachable alert a false positive", "is categraf stuck", "the heartbeat stopped but I can still ping it", etc. Core stance: **an unreachable agent != a down host**. Concluding "down" just because target_up==0 / BeatTime stopped is a common source of false positives.
tags:
  - internal
---

# Host Health Comprehensive Assessment (host-health-diagnose)

## Scope

**Enter this skill**:
- "Why is the machine xxx unreachable / offline / heartbeat stopped"
- "Is the host-unreachable alert a false positive"
- "Is the agent stuck / hung"
- "Is this host really down"
- "This machine is pingable but Nightingale shows it as down"

**Do NOT enter this skill**:
- Change alert rule thresholds, add/adjust host-unreachable alerts → `creation` / `troubleshooting`
- Just "take a look at the machine list / details" → `resource_query`
- "Why did this alert fire" (not host-unreachable type) → `troubleshooting`

## One-Sentence Principle

**A diagnosis that concludes from a single layer of evidence is almost always wrong.**

When n9e shows a machine as unreachable, it may stem from:
- The agent really died (process panic / OOM)
- The agent is still alive but cannot send heartbeats (network partition / DNS / broken proxy / cannot write to Redis)
- Redis write latency on the server side
- The user manually stopped the agent for maintenance but did not add a mute
- The host is really down

The "recommended action" for these five conclusions is completely different. So **gather evidence first, then classify, and give the action last**.

## Three-Layer Evidence Collection (in order, call in parallel)

### Layer 1: Real-time heartbeat and metadata — `get_target_realtime_status(ident)`

Reads Redis to get:
- `beat_time` / `lag_seconds` — seconds since the most recent heartbeat
- `status` — `active`(<60s) / `lagging`(60–180s) / `stale`(≥180s) / `stale_no_heartbeat`(no such key in Redis)
- `offset` — clock skew between agent and server (an abnormally large value usually means the agent clock has drifted / NTP is broken)
- `cpu_util` / `mem_util` — resource usage from the most recent report
- `agent_version` / `remote_addr` / `extend_info`

**Key judgments**:
- `status=stale_no_heartbeat` while `update_at_db` is several days ago — the agent has never connected, a deployment problem.
- `status=stale_no_heartbeat` while `update_at_db` is a few minutes ago — the Redis layer is down or has been cleared.
- `status=stale` while the last reported `cpu_util` / `mem_util` was low — possibly a graceful exit (shutdown / kernel reclaim after OOM), leaning toward "truly down".
- `status=stale` while the last reported `cpu_util` was high (>90%) — leaning toward "agent hung / host hang".
- absolute value of `offset` > 30s — not necessarily down, but the agent clock has a problem and will throw off alert evaluation, so flag it separately.

### Layer 2: Recent 10m metric window — `query_host_metrics_window(ident)`

By default queries `cpu_usage_active` / `mem_available_percent` / `system_load1` / `net_bytes_recv` / `net_bytes_sent`. Returns `samples_count / first_ts / last_ts / min / max / avg / last` for each metric.

**Key judgments**:
- `samples_count=0` and `series=0` — the Prom layer has no data at all. If it stops at the same moment as BeatTime → heartbeat and metrics share the same source (categraf), and stopping together means the agent really is not sending; stopping at different moments means the data flow was cut by different stages.
- `last_ts` far earlier than `now` — metrics have also stopped; compare with BeatTime to see whether the stop times match.
- `samples_count > 0` but BeatTime is already stale — **a typical agent hang or Redis write latency**: the metric stream is still flowing (the categraf main process is sending), but the heartbeat cannot be written to Redis.
- `cpu_usage_active.last` > 95% + `system_load1.last` far above cpu_num — a strong signal of host hang.
- `net_bytes_recv.last + net_bytes_sent.last` close to 0 — the NIC is silent; combined with a stopped BeatTime this points to "truly down" or "network cut".

⚠️ By default, if `datasource_id` is not passed, the chat-level datasource is used as a fallback; when the user has not selected a datasource in the frontend, first `list_datasources` and pick a Prom datasource.

### Layer 3: Neighbors in the same business group — `list_neighbor_targets(ident)`

Pulls other machines in the same **business group** (`target.GroupIds`), returning `items` + `summary{total, active, lagging, stale}`.

**Key judgments**:
- summary almost entirely active — an individual fault; the problem is on this one machine.
- summary mostly stale — a **cluster-level** event: switch, data center, underlying cloud, Redis, or the server heartbeat channel. In this case the question "why is this machine unreachable" is itself the wrong question; you should investigate the cluster event.
- summary half active half stale — possibly a network partition or availability-zone failure; check whether the stale machines are on the same subnet / the same rack.

## Correlation Signals (call as needed)

- `list_alert_mutes` / `get_alert_mute_detail` — matched a mute rule → the "under maintenance" branch; not a false positive, and tell the user when the mute window ends.
- `search_active_alerts` — check whether there are other companion alerts (cpu/mem/disk alerts for the same ident firing at the same time) → leans toward "the host is really in trouble".
- `get_target_detail` — check `update_at` (the most recent time a heartbeat was persisted at the DB layer). If `update_at` is newer than Redis's `beat_time`, the Redis layer has been cleared; the reverse means the DB layer is stuck.

## Three-State Decision Table

| Signal combination | Conclusion | Confidence |
|---|---|---|
| BeatTime stale + metrics also stopped (last_ts ≈ beat_time) + neighbors mostly active + last cpu/mem is low | **Truly down** | High |
| BeatTime stale + metrics also stopped + multiple machines in the same business group stale at once | **Cluster event** (not a single-machine outage; guide them to investigate upstream) | High |
| BeatTime stale + metrics still moving (samples_count > 0 and last_ts close to now) | **Agent hung / Redis heartbeat channel anomaly** | High |
| BeatTime lagging 30–180s + a few neighbors lagging in sync | **Network flapping / brief partition**, will most likely self-heal | Medium |
| Matched a mute / maintenance window | **Under maintenance** | High |
| BeatTime stale + last cpu_util > 95% / load spiking + neighbors normal | **Host hang** (CPU at 100%, IO stuck, kernel softlockup), counts as "down in a broad sense" but the recommended direction differs | Medium |
| None of the above match | **Insufficient data**; honestly say "cannot determine", and list the evidence already collected so the user can fill in the gaps | — |

## Output Template (strong constraint)

Final Answer in Markdown, in the user's language. **Four sections**:

```
## Conclusion
<one sentence: truly down / agent hung / network flapping / under maintenance / cluster event / insufficient data, cannot determine>

## Key Evidence
- Heartbeat: beat_time=2026-05-14 10:23:11, lag=312s, status=stale; db update_at=...
- Metrics (10m): cpu_usage_active recent last=0.5% (steadily declining from first to last value), net_bytes_sent last=0 — metric stream and heartbeat stopped at the same time
- Neighbors: same business group 18/20 active, only this machine stale
- Mute: not matched

## Recommended Actions
1. <the thing to do first>
2. <next best>
3. <if you want to self-verify, do it this way>

## False-Positive Risk
<under what circumstances this conclusion would not hold, e.g. "if there was a recent cleanup operation on the Redis layer, this judgment does not hold">
```

## Anti-Patterns (do NOT do these)

- ❌ Concluding "down" from the single `BeatTime` field. Always pull the metric window and neighbors for cross-validation.
- ❌ Reporting "individual fault" and "cluster fault" mixed together. When all neighbors are stale, do not keep focusing on the single machine; clearly tell the user "you asked the wrong question, it is a cluster event".
- ❌ Recommending "immediately page someone to check the machine" without looking at the mute. A common pitfall: an under-maintenance alert mistaken for a false positive and reported as one.
- ❌ Using `query_prometheus` to pull back a long string of raw time-series points and blow up the token budget. `query_host_metrics_window` already compresses by "min/max/avg/last"; see whether the aggregate is enough before deciding whether to look at the detail.
- ❌ Not telling the user how to verify. The recommended actions must include at least one "if you want to self-verify whether the agent is really dead, do this" step (remotely ssh in and check `systemctl status categraf` / `ps -ef | grep categraf` / `journalctl -u categraf --since "5 min ago"`).

## Known Failure Modes

- edge unreachable storm: under high server load the heartbeat channel develops batch latency, which looks like the whole cluster going offline at once, but is actually Redis write blocking. **When you see summary entirely stale, proactively raise this possibility.**
- Heartbeat update lag: the goroutine that writes heartbeats to Redis is stuck on slow IO; the agent is sending and the server is receiving, but BeatTime is not updated.
- redis nil: the heartbeat key is occasionally cleared/evicted; check this together with the maxmemory-policy configuration.

## Output Style

- No teasing. Put the conclusion on the first line of the first section.
- Give concrete numbers in the evidence; do not write things like "the metrics look normal".
- "Recommended actions" must be executable; avoid empty phrases like "strengthen monitoring".
- Answer in the user's language (Chinese for Chinese users, English for English users).
