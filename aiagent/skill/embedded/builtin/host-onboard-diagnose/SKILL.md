---
name: host-onboard-diagnose
description: 排查"categraf 已装/在跑但夜莺机器列表看不到、或显示 unknown / 无指标"的接入失败问题。当用户问"新装的机器为什么没出现"、"机器列表 OS 都是 unknown"、"Helm 装了 3 个采集器只看到 1 个"、"agent 注册不进来"、"装完 categraf 主机没显示"时触发。与 host-health-diagnose **互斥**：那个处理"曾经接入过、现在失联"，本 skill 处理"压根没接入进来"。核心立场：**机器没出现不是一个原因，而是接入链路上某一段断了**。只看 heartbeat.enable 一项就让用户改 categraf，是常见翻车点（很多用户改了还是看不到，因为问题在 omit_hostname / ident shell / TLS / token / edge redis / 多集群路由）。
max_iterations: 18
builtin_tools:
  - probe_target_onboard_status
  - list_targets
  - get_target_detail
  - query_prometheus
  - query_host_metrics_window
  - list_datasources
tags:
  - internal
---

# 主机接入失败诊断（host-onboard-diagnose）

## 适用范围

**进本 skill**：
- "新装的 categraf 机器在夜莺看不到"
- "agent 装了在跑，但机器列表里没出现"
- "机器列表里这台 OS / CPU / 版本 全是 unknown"
- "Helm 部了 3 台 categraf，平台只看到 1 个"
- "Windows 装了 agent 注册不进来"
- "刚改了 hostname 后机器消失了"（如果伴随 categraf 在跑）

**不进本 skill**：
- 曾经能看到、最近失联 → `host-health-diagnose`
- ident 重名 / 改名后想清理残留 → `host-ident-cleanup`（待建）
- 想改告警规则 / 屏蔽 → `creation` / `create-alert-rule`
- 看告警没发出来为啥 → `alert-rule-troubleshoot`

## 一句话原则

**机器没出现 ≠ 一个原因。** 接入链路有 5 段，每段卡住的表现都不一样，只看一段就让用户改配置是常见翻车点。**先取证、再分段定位、最后给修复命令。**

## 接入链路 5 段

```
[1] categraf 本机进程     在不在 / 配置对不对 / heartbeat.enable 开没
        │
[2] 心跳上报 HTTP          能不能打到 /v1/n9e/heartbeat（网络 / TLS / BasicAuth）
        │
[3] server / edge 接收     token / 版本兼容 / hostname 重名校验
        │
[4] target 表落库          DB 里有没有这条 ident，redis 里有没有 meta
        │
[5] Redis + 指标流          时序库能不能查到 ident 的样本
```

## 第一动作：调 `probe_target_onboard_status`

这是本 skill **唯一**的诊断入口工具，一次性返回 5 段足迹。**永远先调它**，再决定下一步。

返回的关键字段：
- `in_target_db` + `target.os` + `target.agent_version` → 段 3/4 证据
- `in_redis_beat` + `redis_meta.hostname` + `redis_meta.remote_addr` → 段 4 证据
- `in_prom_target_up` + `target_up_last` + `prom_metrics_hit` → 段 5 证据
- `likely_segment` + `likely_causes` → **工具层已聚合的诊断**，**不要绕过它自己重推**

如果用户没给 ident，先 `list_targets` 让用户挑，或者按 OS=unknown / agent_version 空过滤出候选（"未分配 / 半接入"视图）。

## 决策表（按 likely_segment 分支）

| likely_segment | 含义 | 优先建议的修复动作 |
|---|---|---|
| `segment_1_or_2` | DB / redis / prom 都没这台机器 | 在目标机器上：`systemctl status categraf` → `journalctl -u categraf --since "5 min ago"` → 看是否报 `connection refused` / `x509` / `401` |
| `segment_3` | target 有但 OS/agent_version 空 | 检查 categraf 的 `config.toml`：`[heartbeat] enable=true` 且 `omit_hostname=false`；版本 ≥ v0.2.35 |
| `segment_4` | target 落库但 redis 没数据 | 检查 n9e/edge 是否配 redis；edge 模式下 `edge.toml` 的 `[Redis]`；n9e 与 n9e-edge 版本是否一致 |
| `segment_5` | redis 有 beat 但 prom 查不到 | 检查 categraf `[[writers]]` 是否配置；多集群部署时数据源是否走对；ident 是否含 `()` `[]` `*` 等特殊字符 |
| `ok` | 接入正常 | 如用户仍坚持"看不到"，引导他刷新页面 / 检查业务组过滤 / 检查浏览器缓存 |

## 段 5 的三条变体查询（用 `query_prometheus`）

当 `likely_segment=segment_5` 时，**必须**用 `query_prometheus` 跑下面 3 条查询确认是 ident 标签问题还是真的没数据：

```promql
# 1. 标准 ident 等值匹配（最常用）
target_up{ident="<ident>"}

# 2. 模糊匹配（ident 带 IP 前缀 / 别名时用）
target_up{ident=~".*<host>.*"}

# 3. 极端兜底：是否走到了 instance 标签（snmp / 自定义 tag 场景）
{instance=~".*<host>.*"}
```

三条都没数据 → 数据流确实没到 prom，回查 categraf writers / TLS / n9e ingest 队列。
只有 (2) 或 (3) 有数据 → **ident 标签问题**（特殊字符 / global_labels 覆盖 / snmp agent_host_tag 误用），引导用户走 `host-ident-cleanup`（待建）或修 categraf 配置。

## 输出模板（强约束）

Final Answer 用 Markdown，用户语言。**四段**：

```
## 结论
<一句话：卡在第 X 段：xxxx（或：接入正常，看不到是 yyy 原因）>

## 接入链路证据
- 段 1/2（categraf 本机/HTTP）：<未取证 / 推断异常：xxx>
- 段 3（server 接收）：target in_db=true, os=unknown, agent_version="" → heartbeat 元数据未落
- 段 4（target 落库 + redis）：target update_at=2026-05-14 10:23:11 但 redis 无心跳
- 段 5（Prom）：target_up 无数据 / prom_metrics_hit=0

## 修复命令
1. 在目标主机执行：
   `grep -E 'heartbeat|omit_hostname' /etc/categraf/conf/config.toml`
   预期：heartbeat 段 enable=true，omit_hostname=false。任一不符合就改并 `systemctl restart categraf`。
2. ...
3. ...

## 自证步骤
<给用户"如果你想确认修好了，可以这样验证"的 1-2 条命令，例如：
- 重启 categraf 后 30s 内回平台刷新机器列表，应能看到 OS/CPU 字段非 unknown
- `curl -s http://<n9e>:17000/api/n9e/self-metrics | grep <ident>`>
```

## 反模式（这些不要做）

- ❌ 不调 `probe_target_onboard_status` 直接让用户改 `heartbeat.enable`。**先取证**。许多用户 heartbeat 已经开了，问题在 omit_hostname / TLS / 版本。
- ❌ 只看 `in_target_db=false` 就说"categraf 没装"。要看 redis 段和 segment_1_or_2 的 causes 综合判断，是网络问题还是进程问题，建议动作完全不同。
- ❌ 段 3 卡住时只让用户改 heartbeat，不提 omit_hostname / 版本。这两条同样常见。
- ❌ 段 5 卡住时不跑 3 条变体 PromQL 就让用户改 writers。ident 标签问题也会卡在段 5。
- ❌ 输出里不给具体命令，只说"检查 categraf 配置"。每条建议必须是用户能直接粘贴执行的。

## 各段已知故障形态

- **段 1/2**：categraf 连不上 center、连接拒绝、TLS unknown authority、自签证书、BasicAuth 失效、ams token 不匹配、Helm 多节点只见 1、Windows、Win2008 不支持
- **段 3**：heartbeat enable=false、unknown 字段 / omit_hostname=true、categraf 版本过低、v6 需 v0.2.35+、identity shell 取 IP 失败、hostname 重名
- **段 4**：edge redis nil、n9e 与 n9e-edge 版本不一致、CenterApi missing、edge 部署中心看不到机器
- **段 5**：ident 带括号大盘查不到、host=* bug、snmp 用 ident 冲突、omit_hostname=true 导致 ident 标签丢失、多集群数据源走错、write queue full 499、global.labels 覆盖

## 输出风格

- 不卖关子。结论第一段第一行。
- 证据要给具体字段值，不要写"看起来正常"。
- 修复命令必须可粘贴执行，避免"检查一下"这种废话。
- 用户语言回答（中文用户中文，英文用户英文）。
- 如果 `likely_segment=ok` 但用户坚持机器没出现，提示去查：业务组过滤（机器在但前端按业务组隐藏）、浏览器缓存、登录用户的可见业务组权限。
