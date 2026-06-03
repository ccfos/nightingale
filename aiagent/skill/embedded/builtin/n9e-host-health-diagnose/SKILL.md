---
name: n9e-host-health-diagnose
description: 帮用户判断一台机器到底是 真宕机 / agent 假死 / 网络抖动 / 维护中。当用户问"为什么这台机器失联"、"host 失联告警是不是误报"、"categraf 卡住了吗"、"心跳停了为啥还能 ping 通"等触发本技能。核心立场：**agent 失联 ≠ 主机宕机**。只看 target_up==0 / BeatTime 停就下"宕机"结论，是常见的误报根源。
---

# 主机健康综合判断（host-health-diagnose）

## 适用范围

**进本 skill**：
- "为什么 xxx 这台机器失联 / 离线 / 心跳停了"
- "host 失联告警是不是误报"
- "agent 是不是卡死了 / 假死"
- "这台主机是不是真的宕机了"
- "这台机器 ping 得通但夜莺显示 down"

**不进本 skill**：
- 改告警规则阈值、加/调 host 失联告警 → `creation` / `troubleshooting`
- 只是"看一下机器列表 / 详情" → `resource_query`
- "为什么这条告警触发了"（非 host 失联类）→ `troubleshooting`

## 一句话原则

**只看一层证据就下结论的诊断，几乎都是错的。**

n9e 显示一台机器失联，可能来自：
- agent 真挂了（进程 panic / OOM）
- agent 还活着但发不出心跳（网络分区 / DNS / 代理坏了 / Redis 写不进去）
- server 端 Redis 写延迟（issue #1589 / #1888 / #2829）
- 用户手动停了 agent 做维护，但没加屏蔽
- 主机真宕了

这五种结论的"建议动作"完全不一样。所以**先取证、再分类、最后给动作**。

## 三层证据采集（按顺序、并行调用）

### 第 1 层：实时心跳与元数据 — `get_target_realtime_status(ident)`

读 Redis 拿：
- `beat_time` / `lag_seconds` — 距离最近一次心跳秒数
- `status` — `active`(<60s) / `lagging`(60–180s) / `stale`(≥180s) / `stale_no_heartbeat`(Redis 里没有这个 key)
- `offset` — agent 与 server 时钟偏移（异常大通常意味着 agent 时钟跑飞 / NTP 坏）
- `cpu_util` / `mem_util` — 最近一次上报的资源占用
- `agent_version` / `remote_addr` / `extend_info`

**关键判断**：
- `status=stale_no_heartbeat` 而 `update_at_db` 是几天前 — agent 从来没接通过，部署问题。
- `status=stale_no_heartbeat` 而 `update_at_db` 是几分钟前 — Redis 这层挂了或被清理，issue #1888 那条路径。
- `status=stale` 而 `cpu_util` / `mem_util` 上一次上报的是低位 — 可能是优雅退出（关机 / OOM 后内核回收），偏向"真宕机"。
- `status=stale` 而 `cpu_util` 上一次上报的是高位（>90%）— 偏向"agent 假死 / 主机 hang"。
- `offset` 绝对值 > 30s — 不一定宕机，但 agent 时钟有问题，会拖垮告警判定，要单独提示。

### 第 2 层：最近 10m 指标窗口 — `query_host_metrics_window(ident)`

默认查 `cpu_usage_active` / `mem_available_percent` / `system_load1` / `net_bytes_recv` / `net_bytes_sent`。返回每个指标的 `samples_count / first_ts / last_ts / min / max / avg / last`。

**关键判断**：
- `samples_count=0` 且 `series=0` — Prom 这一层完全没数据。和 BeatTime 一起停在同一时刻 → 心跳和指标同源（categraf），同时停说明 agent 真没在发；不同时刻停说明数据流被不同环节切断。
- `last_ts` 远早于 `now` — 指标也停了，配合 BeatTime 看停止时间是否一致。
- `samples_count > 0` 但 BeatTime 已 stale — **典型 agent 假死或 Redis 写延迟**：指标流还在（categraf 主进程在发），但心跳写不进 Redis。这是 issue #1589 的形态。
- `cpu_usage_active.last` > 95% + `system_load1.last` 远超 cpu_num — 主机 hang 的强信号。
- `net_bytes_recv.last + net_bytes_sent.last` 接近 0 — 网卡静默，结合 BeatTime 停止可推"真宕机"或"网络断"。

⚠️ 默认 `datasource_id` 不传会用 chat-level 的 datasource 兜底；用户没在前端选数据源时要先 `list_datasources` 选一个 Prom 数据源。

### 第 3 层：同业务组邻居 — `list_neighbor_targets(ident)`

按 **业务组**（`target.GroupIds`）拉同组其它机器，返回 `items` + `summary{total, active, lagging, stale}`。

**关键判断**：
- summary 几乎全 active — 个体故障，问题在这一台。
- summary 大半 stale — **集群级**事件：交换机、机房、底层云、Redis、server 心跳通道。这种情况下"这台为什么失联"这个问题本身就问错了，应该排集群事件。
- summary 一半 active 一半 stale — 可能是网络分区、可用区故障，看 stale 的几台是不是同一个网段 / 同一个 rack。

## 关联信号（按需调）

- `list_alert_mutes` / `get_alert_mute_detail` — 命中屏蔽规则 → "维护中"分支，不算误报，提示用户屏蔽窗口何时结束。
- `search_active_alerts` — 看是不是还有其它伴生告警（同 ident 的 cpu/mem/disk 告警同时在烧）→ 偏"主机真出事"。
- `get_target_detail` — 看 `update_at`（DB 层最近一次心跳落库时间）。如果 `update_at` 比 Redis 的 `beat_time` 新，是 Redis 这一层被清掉了；反过来则是 DB 这一层卡了。

## 三态决策表

| 信号组合 | 结论 | 置信度 |
|---|---|---|
| BeatTime stale + 指标也停（last_ts ≈ beat_time）+ 邻居 active 占多 + 最后 cpu/mem 是低位 | **真宕机** | 高 |
| BeatTime stale + 指标也停 + 同业务组多台同时 stale | **集群事件**（不是单台宕机，引导排上游） | 高 |
| BeatTime stale + 指标还在动（samples_count > 0 且 last_ts 接近 now）| **agent 假死 / Redis 心跳通道异常**（issue #1589/#1888） | 高 |
| BeatTime lagging 30–180s + 邻居有几台同步 lagging | **网络抖动 / 短暂分区**，多半会自愈 | 中 |
| 命中 mute / 维护窗口 | **维护中** | 高 |
| BeatTime stale + 最后 cpu_util > 95% / load 飙高 + 邻居正常 | **主机 hang**（CPU 100%、IO 卡、kernel softlockup），算"广义宕机"但建议方向不一样 | 中 |
| 上面都对不上 | **数据不足**，老老实实说"无法判断"，列出已采集的证据让用户补充 | — |

## 输出模板（强约束）

Final Answer 用 Markdown，用户语言。**四段**：

```
## 结论
<一句话：真宕机 / agent 假死 / 网络抖动 / 维护中 / 集群事件 / 数据不足无法判断>

## 关键证据
- 心跳：beat_time=2026-05-14 10:23:11，lag=312s，status=stale；db update_at=...
- 指标（10m）：cpu_usage_active 最近 last=0.5%（首→末值持续走低），net_bytes_sent last=0 — 指标流与心跳同时停
- 邻居：同业务组 18/20 active，stale 仅本机
- 屏蔽：未命中

## 建议动作
1. <最该先做的事>
2. <次优>
3. <如果想自证，可以这样验证>

## 误报风险
<本结论在什么情况下会站不住脚，例如"如果 Redis 这一层最近有清理操作，本判断不成立">
```

## 反模式（这些不要做）

- ❌ 只看 `BeatTime` 一个字段就说"宕机"。永远要拉指标窗口和邻居作交叉验证。
- ❌ 把"个体故障"和"集群故障"混着报。邻居全 stale 时不要继续聚焦单台，要明确告诉用户"问错了，是集群事件"。
- ❌ 没看 mute 就给"立刻拉人查机器"的建议。社区高频翻车点：维护中告警被当成误报投诉。
- ❌ 用 `query_prometheus` 拉一长串原始时序点回来 token 爆掉。`query_host_metrics_window` 已经按"min/max/avg/last"做过压缩，先看聚合够用了再决定是否要细看。
- ❌ 不告诉用户验证方法。建议动作里至少给一条"如果你想自证 agent 是不是真死了，可以这样做"的步骤（远程 ssh 进去看 `systemctl status categraf` / `ps -ef | grep categraf` / `journalctl -u categraf --since "5 min ago"`）。

## 相关 issue

- #2829 edge 失联风暴：心跳通道在 server 高负载下出现批量延迟，看着像集群同时下线，其实是 Redis 写阻塞。**遇到 summary 整片 stale 时主动提这个可能性。**
- #1589 心跳更新滞后：Redis 写心跳的 goroutine 被慢 IO 卡住，agent 在发 server 在收但 BeatTime 不更新。
- #1888 redis nil：心跳 key 偶发被清空/驱逐，配合 maxmemory-policy 配置看。

## 输出风格

- 不卖关子。结论放第一段第一行。
- 证据要给具体数字，不要写"指标看起来正常"这种话。
- "建议动作"必须可执行，避免"加强监控"这种废话。
- 用户语言回答（中文用户中文，英文用户英文）。
