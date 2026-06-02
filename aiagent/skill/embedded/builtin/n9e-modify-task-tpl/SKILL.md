---
name: n9e-modify-task-tpl
description: 帮助用户生成、修改或排障夜莺(n9e)告警自愈脚本（task_tpl / ibex 脚本）。当用户要求"写一个磁盘清理/重启服务/清理日志/dump 进程/reload nginx"等自愈脚本，或问"自愈脚本怎么拿告警传过来的参数"、"stdin 是什么格式"、"timeout 应该填多少"、"为什么 is_recovered 永远 false"、"为什么自愈脚本拿不到 k8s namespace"、"脚本一直 running 怎么办"时使用。本技能专注**脚本正文层**——若用户要改告警规则、接收人或通知模板，引导到对应 skill。
tags:
  - internal
---

# 夜莺(n9e) 告警自愈脚本（task_tpl）生成

夜莺告警自愈是 **ibex 子系统**：告警规则的 `callbacks` 字段写成 `${ibex}/<task_tpl_id>` 时，告警事件触发后会按当前 event 的 `TargetIdent` 拉起对应 task_tpl 的脚本，在那台机器的 categraf 上执行。脚本通过 **stdin** 拿到本次告警的标签。

本技能专注**写/改 `task_tpl.script` 字段本身**——不涉及创建告警规则、配置接收人或编辑通知模板。

---

## 1. 适用范围：先确定用户在改哪一层

夜莺告警链路分四层，每层走不同的 skill：

| 层 | 实体 | 关键文件 | 本 skill 是否管 |
|---|---|---|---|
| **自愈脚本** task_tpl | `task_tpl` 表 | `models/task_tpl.go`、`alert/sender/ibex.go` | **是** |
| 告警规则 | `alert_rule` 表 | `models/alert_rule.go` | 否（用 `n9e-create-alert-rule`） |
| 通知模板 | `notify_tpl` 表 | `models/notify_tpl.go` | 否（用 `n9e-generate-message-template`） |
| 通知通道 | `notify_channel` 表 | `models/notify_channel.go` | 否（用 `n9e-notify-channel-copilot`） |

**判断口径**：用户原话出现"脚本/shell/bash/python/jq/解析/执行/超时"——本 skill；出现"PromQL/阈值/触发条件"——告警规则；出现"模板/正文/字段渲染"——消息模板；出现"URL/Webhook/签名"——通知通道。

---

## 2. 数据模型（用户能填什么、夜莺真用哪些）

### `TaskTpl`（`models/task_tpl.go:17-35`）

| 字段 | 类型 | 含义 |
|---|---|---|
| `id` | int64 | 主键，告警规则 callbacks 里 `${ibex}/<id>` 引用的就是它 |
| `group_id` | int64 | 业务组（权限边界，`CanDoIbex` 会校验） |
| `title` | string | 模板名，执行时会拼成 `<title> FH: <hostname>` 写到任务标题 |
| `script` | string | **本技能主要操作的字段** |
| `args` | string | 命令行参数。告警触发执行时如果调用者没传 `args`，用这里的默认值 |
| `tags` | string | 空格分隔的标签，模板列表筛选/分类用，不影响执行 |
| `account` | string | 在目标机器上以哪个用户身份运行（如 `root`） |
| `batch` | int | 每批并发主机数。**自愈通常只跑触发那台，0 即可** |
| `tolerance` | int | 批内允许失败数。单机自愈场景留 0 |
| `timeout` | int | **秒**。0 → 默认 30；> 5 天 → 拒绝 |
| `pause` | string | 批次间暂停时间表（cron 风格）。自愈基本不用 |

### `TaskForm`（`models/task_tpl.go:351-365`）—— 真正下发时的载体

告警触发自愈走 `alert/sender/ibex.go CallIbex` → 构造 `TaskForm` → `TaskAdd` 写入 ibex：

- `AlertTriggered: true` —— 边缘机房会走 Redis 直发 categraf，不依赖中心 DB（`ibex.go:244-276`）
- `Stdin: <JSON 字符串>` —— **本技能最关心**，下一节详解
- `Hosts: []string{event.TargetIdent}` —— 只在告警事件命中的那台机器执行
- `Title: tpl.Title + " FH: " + host` —— 执行历史里好认（FH = From Host）

### CleanFields 硬约束（`models/task_tpl.go:137-189`）

保存或下发前会做这些校验，违反会被直接拒绝：

- `timeout == 0` → 自动设为 30
- `timeout > 3600*24*5` → 报错"longer than five days"
- `title` / `args` / `pause` / `tags` 含 `str.Dangerous` 的字符（`` ` ``、`$()`、`&&` 等）→ 报错
- `script` 自动 `\r\n → \n`（解掉 Windows 编辑器拷贝的 #1713 CRLF 问题）
- `script == ""` → 报错"script is required"

---

## 3. ⚠️ stdin 的真相（最容易踩的坑，**必读**）

### 3.1 stdin 是 `map[string]string` 的扁平 JSON，**不是 `$event` 对象**

很多用户从消息模板文档抄了 `{{$event.RuleName}}` 这种语法过来，**完全不能用**。`$event` 是 `notify_tpl` 模板的上下文，ibex 自愈脚本里没有 template 渲染过程——它拿到的就是一段纯 JSON。

**stdin 的构造逻辑**（`alert/sender/ibex.go:118-142`）：

```go
tagsMap := make(map[string]string)
for _, pair := range event.TagsJSON {        // event 标签一条条展平
    k, v := splitOnce(pair, "=")
    tagsMap[k] = v
}
// 注入 3 个内置 key
tagsMap["alert_severity"]      = strconv.Itoa(event.Severity)
tagsMap["alert_trigger_value"] = event.TriggerValue
tagsMap["is_recovered"]        = strconv.FormatBool(event.IsRecovered)

tags, _ := json.Marshal(tagsMap)             // 整体序列化为 string
in.Stdin = string(tags)
```

**真实 stdin payload 示例**：

```json
{
  "ident": "host01",
  "instance": "host01:9100",
  "job": "categraf",
  "alert_severity": "2",
  "alert_trigger_value": "92.5",
  "is_recovered": "false"
}
```

注意：所有值都是字符串（`alert_severity` 是 `"2"` 不是 `2`），脚本里要 parse 成数字得自己转。

### 3.2 三条"想拿但拿不到"的字段及解法

| 想拿的字段 | 为什么拿不到 | 解法 |
|---|---|---|
| `rule_name` / `rule_id` / `trigger_time` / `severity` 中文名 | stdin 只装 labels，没装 event 元数据 | PromQL 里用 `label_replace(..., "rule_name", "...", "...", "...")` 把信息注入到标签；或者改用 callback 媒介（HTTP 传整个 event JSON） |
| 接收人 / 通知组 | 自愈与通知规则是**两条独立通道**，self_heal 只看 `alert_rule.callbacks`，不看 `notify_rule` | 想用接收人 → 走 callback 媒介，stdin 不是这条路 |
| 被 PromQL 聚合掉的标签（如 k8s `namespace`/`deployment`） | `sum(...)` 不带 `by(...)` 会丢标签 | **告警规则里 PromQL 要写成 `sum by (instance, namespace, deployment)(...)`**，被保留的标签才会进 stdin |

### 3.3 三语言读 stdin 模板（生成脚本时必须包含）

**shell + jq（最常见）**：

```bash
#!/bin/bash
set -euo pipefail

PAYLOAD=$(cat)                    # 一次性读完 stdin，避免阻塞
[ -z "$PAYLOAD" ] && { echo "FATAL: empty stdin payload"; exit 2; }

IDENT=$(echo "$PAYLOAD" | jq -r '.ident          // empty')
SEV=$(echo   "$PAYLOAD" | jq -r '.alert_severity // "3"')
VAL=$(echo   "$PAYLOAD" | jq -r '.alert_trigger_value // "0"')

[ -z "$IDENT" ] && { echo "FATAL: no ident in stdin"; exit 2; }
echo "[$(date -Iseconds)] ident=$IDENT severity=$SEV value=$VAL"
```

**shell 无 jq 兜底**（用 grep+sed，适合最小化容器）：

```bash
PAYLOAD=$(cat)
IDENT=$(echo "$PAYLOAD" | grep -oE '"ident"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*"\([^"]*\)"$/\1/')
```

**python**（处理逻辑复杂时）：

```python
#!/usr/bin/env python3
import sys, json, time

try:
    data = json.load(sys.stdin)
except json.JSONDecodeError as e:
    print(f"FATAL: bad stdin json: {e}", file=sys.stderr)
    sys.exit(2)

ident = data.get("ident", "")
sev   = int(data.get("alert_severity", "3"))
val   = float(data.get("alert_trigger_value", "0"))

if not ident:
    print("FATAL: no ident", file=sys.stderr)
    sys.exit(2)

print(f"[{time.strftime('%FT%T')}] ident={ident} sev={sev} val={val:.2f}")
```

**go**（极少用，给最小骨架）：

```go
package main

import (
    "encoding/json"
    "fmt"
    "os"
)

func main() {
    var d map[string]string
    if err := json.NewDecoder(os.Stdin).Decode(&d); err != nil {
        fmt.Fprintln(os.Stderr, "FATAL:", err)
        os.Exit(2)
    }
    fmt.Println("ident:", d["ident"])
}
```

---

## 4. ⚠️ `is_recovered` 的真相（**别写死代码**）

**ibex 自愈不会在恢复事件触发**——`alert/sender/ibex.go:39-42`：

```go
func (c *IbexCallBacker) CallBack(ctx CallBackContext) {
    ...
    if event.IsRecovered {
        logger.Infof("event_callback_ibex: event is recovered, event: %s", event.Hash)
        return                       // ← 直接返回，根本不调 handleIbex
    }
    ...
}
```

**所以 stdin 里的 `is_recovered` 永远是字符串 `"false"`。** 写 `if [ "$IS_RECOVERED" = "true" ]; then ...` 是死代码。

**正确路径**：

- 想"告警恢复时也跑个动作（发邮件 / 关工单 / 通知 IM）" → 用 **notify_rule + callback 媒介**，在 notify_rule 里勾选"恢复也通知"，callback URL 收到事件 JSON 后再处理。
- 想"恢复事件触发另一段脚本" → 同上，**不要**试图复用 task_tpl。

历史 issue（lucky0137 #2211）的核心痛点是"恢复也想触发"，文档里写"加 is_recovered 判断"是无效解——本质上需要架构层改 ibex 接受恢复事件，或者用户改走 callback 通道。

---

## 5. timeout / batch / tolerance / pause —— 数字字段语义

### 5.1 timeout（秒）

- 默认 30 秒（`CleanFields` 自动填）
- 最长 5 天（更长直接报错，#1950 维护者明确"自愈 = 短周期任务"）
- 超时后**进程会被 SIGKILL**，stdout/stderr 在被 kill 之前已写到 task_host 表
- **与告警评估 interval 没有直接关系**——评估周期 60s 不意味着脚本只有 60s 可跑

**典型值参考**：

| 场景 | 建议 timeout |
|---|---|
| reload 配置 / systemctl restart | 30 ~ 60 |
| 清理日志 / 镜像缓存（数 GB 级） | 120 ~ 300 |
| jstack/jmap 取 heap dump | 300 ~ 600 |
| yum/apt 安装包 | 600 ~ 1800 |

### 5.2 区分三个"超时类"历史 issue 的根因

| Issue | 现象 | 真根因 | 解法 |
|---|---|---|---|
| #864 | docker-compose 自愈脚本超时 | 误把 telegraf 当 ibex-agent，**通道根本没建立** | 装 categraf（不是 telegraf），看 `categraf -test` 输出 |
| #1504 | 通知脚本 timeout=0 立即被 kill | fe 表单 DefaultTimeout=0 BUG | 后端 CleanFields 已修（0 → 30） |
| #2596 | 自愈脚本执行超时 | 脚本本身慢（远程 yum / 大文件拷贝） | 调大 `timeout`；考虑改用 p2p 工具传文件（不要拿 ibex 当文件分发） |

**不要混为一谈**——用户说"超时"先问场景。

### 5.3 batch / tolerance / pause

- **batch**：每批并发主机数。告警触发的单机自愈用不上，留 0 即可。手动批量任务（运维场景）才用。
- **tolerance**：批内允许失败数。同 batch，自愈场景留 0。
- **pause**：批次间暂停（如 `00:00-08:00,17:00-23:59` 表示这些时间段暂停）。自愈基本用不到——告警随时可能触发，没法靠 pause 做工作时段限制。**真的要限制工作时段**应该在 alert_rule 的 `enable_in_bg` / `enable_stime` 字段做，或者用 notify_rule 的时间窗口。

---

## 6. 危险命令清单（生成时必须规避或加护栏）

### 6.1 黑名单（**直接拒绝生成**，即使用户明确要求）

| 命令 | 风险 |
|---|---|
| `rm -rf /` / `rm -rf /*` / `rm -rf $UNSET_VAR/` | 整盘删除 |
| `mkfs.*`、`dd of=/dev/sda`、`shred /dev/sda` | 文件系统破坏 |
| `shutdown`、`reboot`、`init 0`、`init 6`、`halt`、`poweroff` | 整机停机——自愈脚本不应有此权限 |
| `iptables -F` / `ufw disable` / `firewall-cmd --reload` 无备份 | 网络/安全策略丢失 |
| `chmod -R 777 /`、`chown -R nobody:nobody /` | 权限破坏 |
| `curl <非白名单 URL> \| sh`、`wget ... -O - \| bash` | 远程代码注入 |
| base64/zip/gzip 编码的内嵌 shell | 静态审查规避 |
| `kubectl delete node`、`kubectl drain` 无 PDB 检查 | 集群级影响 |

### 6.2 灰名单（**生成但必须加护栏**）

| 命令 | 必须配套的护栏 |
|---|---|
| `systemctl restart <svc>` | lock file 防止重入；记录 PID before/after |
| `kill -9` | 先 `kill -TERM` 等 10s；再 `-9`；记录进程信息 |
| `docker rm` / `docker rmi` | 先 `docker ps -a` 过滤，禁止删 running 容器 |
| `find -delete` | 先 dry-run（`find ... -print | head`），再加 `-mtime +N` 限制范围 |
| `truncate` / `> /var/log/xxx` | 备份到 `.bak.<ts>`；不动 active log（先 logrotate） |
| `iptables -A` | 写入 `/etc/iptables/rules.v4.bak.<ts>` 备份 |

### 6.3 通用护栏模板

```bash
#!/bin/bash
set -euo pipefail

# (1) 单实例锁，防止并发触发
LOCK=/var/run/$(basename "$0").lock
exec 9>"$LOCK"
flock -n 9 || { echo "another instance running, skip"; exit 0; }

# (2) dry-run 开关（args 里传 --dry-run 即不真做）
DRY_RUN=0
[[ "${1:-}" == "--dry-run" ]] && DRY_RUN=1

# (3) before 状态
echo "=== BEFORE ==="
df -h /var/log

# (4) 主体（按 DRY_RUN 切换）
if [[ $DRY_RUN -eq 1 ]]; then
    find /var/log -name "*.log" -mtime +7 -print | head -20
else
    find /var/log -name "*.log" -mtime +7 -delete
fi

# (5) after 状态
echo "=== AFTER ==="
df -h /var/log
```

---

## 7. 内置场景库（命中"想不到好场景"）

每个场景给：典型告警规则名 → 脚本骨架 → timeout 建议 → 风险点。

### 7.1 磁盘空间类

**S1：清理 `/var/log` 下 7 天前的日志**

- 触发：`disk_used_percent{mountpoint="/"} > 90`
- timeout: 120

```bash
#!/bin/bash
set -euo pipefail
PAYLOAD=$(cat)
echo "before: $(df -h /var/log | tail -1)"
find /var/log -type f -name "*.log.*" -mtime +7 -print -delete | wc -l
find /var/log -type f -name "*.gz"    -mtime +7 -print -delete | wc -l
echo "after:  $(df -h /var/log | tail -1)"
```

**S2：清理 docker 镜像层缓存**

- 触发：磁盘满 + 主机有 docker
- timeout: 300

```bash
#!/bin/bash
set -euo pipefail
command -v docker >/dev/null || { echo "no docker, skip"; exit 0; }
echo "before:"; df -h /var/lib/docker
docker image prune -af --filter "until=168h"     # 7 天前的镜像
docker builder prune -af --filter "unused-for=168h"
echo "after:"; df -h /var/lib/docker
```

**S3：清理 `/tmp` 30 天前的文件**

- timeout: 60

```bash
#!/bin/bash
set -euo pipefail
find /tmp -xdev -type f -mtime +30 -print -delete | wc -l
find /tmp -xdev -type d -empty -mtime +30 -delete 2>/dev/null || true
```

**S4：journalctl 缩容到 500MB**

- timeout: 60

```bash
#!/bin/bash
set -euo pipefail
journalctl --disk-usage
journalctl --vacuum-size=500M
journalctl --disk-usage
```

### 7.2 内存 / 进程类

**S5：找出 top5 内存进程并 dump（不自动 kill）**

- 触发：`mem_used_percent > 90`
- timeout: 60
- ⚠️ 故意不杀进程——内存高的根因是业务问题，自动 kill 会扩大故障

```bash
#!/bin/bash
set -euo pipefail
TS=$(date +%Y%m%d-%H%M%S)
OUT=/var/log/n9e-mem-top5.$TS.txt
{
    echo "=== top5 mem proc @$TS ==="
    ps -eo pid,user,%mem,%cpu,etime,cmd --sort=-%mem | head -6
    echo; free -m
} > "$OUT"
echo "dumped to $OUT"
```

**S6：JVM OOM 自动 dump + 重启**

- 触发：`process_jvm_heap_used_percent > 95`
- timeout: 600
- ⚠️ 需要 stdin 里有 `service` 标签（PromQL by 里加上）

```bash
#!/bin/bash
set -euo pipefail
PAYLOAD=$(cat)
SVC=$(echo "$PAYLOAD" | jq -r '.service // empty')
[ -z "$SVC" ] && { echo "no service tag"; exit 2; }

PID=$(systemctl show -p MainPID --value "$SVC")
[ "$PID" = "0" ] && { echo "service not running"; exit 0; }

TS=$(date +%Y%m%d-%H%M%S)
DIR=/var/log/jvm-dump
mkdir -p "$DIR"

jstack "$PID" > "$DIR/jstack.$SVC.$TS.txt" || true
jmap -dump:live,format=b,file="$DIR/heap.$SVC.$TS.hprof" "$PID" || true

# lock file 防止重入
LOCK=/var/run/n9e-restart-$SVC.lock
exec 9>"$LOCK"; flock -n 9 || { echo "already restarting"; exit 0; }

systemctl restart "$SVC"
sleep 5
systemctl is-active "$SVC"
```

**S7：僵尸进程清理**

- 触发：`zombie_process_count > 5`
- timeout: 30

```bash
#!/bin/bash
set -euo pipefail
ps -eo stat,ppid,pid,cmd | awk '$1 ~ /^Z/ {print $2}' | sort -u | while read ppid; do
    echo "send SIGCHLD to ppid $ppid"
    kill -SIGCHLD "$ppid" 2>/dev/null || true
done
```

### 7.3 服务可用性类

**S8：服务端口 down → systemctl restart（带冷却）**

- 触发：`net_tcp_connection_count{port="3306"} < 1`
- timeout: 120

```bash
#!/bin/bash
set -euo pipefail
PAYLOAD=$(cat)
SVC=$(echo "$PAYLOAD" | jq -r '.service // empty')
[ -z "$SVC" ] && { echo "no service tag"; exit 2; }

# 冷却：5 分钟内已重启过则跳过
COOLDOWN=/var/run/n9e-restart-$SVC.cooldown
if [ -f "$COOLDOWN" ] && [ $(($(date +%s) - $(stat -c%Y "$COOLDOWN"))) -lt 300 ]; then
    echo "in cooldown, skip"
    exit 0
fi

systemctl restart "$SVC"
touch "$COOLDOWN"
sleep 5
systemctl is-active "$SVC"
```

**S9：nginx 配置变更后 reload**

- 触发：手动触发 / `file_change_count{path="/etc/nginx"} > 0`
- timeout: 30

```bash
#!/bin/bash
set -euo pipefail
nginx -t                          # 验证配置
nginx -s reload
sleep 1
pgrep -x nginx >/dev/null || { echo "nginx died after reload"; exit 1; }
echo "reloaded ok"
```

**S10：crashloop 容器只诊断不重启（k8s）**

- 触发：`kube_pod_container_status_restarts_total > 5`
- timeout: 60
- ⚠️ k8s 已经会自动重启，自愈只做诊断 + 通知

```bash
#!/bin/bash
set -euo pipefail
PAYLOAD=$(cat)
NS=$(echo "$PAYLOAD" | jq -r '.namespace // "default"')
POD=$(echo "$PAYLOAD" | jq -r '.pod // empty')
[ -z "$POD" ] && { echo "no pod tag (check PromQL by clause)"; exit 2; }

TS=$(date +%Y%m%d-%H%M%S)
OUT=/var/log/n9e-crash-$NS-$POD.$TS.log
{
    kubectl -n "$NS" describe pod "$POD"
    echo "==="
    kubectl -n "$NS" logs "$POD" --tail=200 --previous || true
} > "$OUT"
echo "diagnosed → $OUT"
```

### 7.4 网络 / 系统类

**S11：网卡 down → ip link set up（白名单）**

- 触发：`net_interface_up{iface=~"eth.+|ens.+"} == 0`
- timeout: 30

```bash
#!/bin/bash
set -euo pipefail
PAYLOAD=$(cat)
IFACE=$(echo "$PAYLOAD" | jq -r '.iface // empty')
[ -z "$IFACE" ] && { echo "no iface"; exit 2; }

# 白名单：只允许 eth* / ens* / eno*，避免动到容器/VPN 网卡
[[ "$IFACE" =~ ^(eth|ens|eno) ]] || { echo "iface $IFACE not in whitelist"; exit 0; }

ip link show "$IFACE"
ip link set "$IFACE" up
sleep 2
ip link show "$IFACE"
```

**S12：NTP 偏移 → 重启 chronyd**

- 触发：`system_clock_offset_seconds > 60`
- timeout: 30

```bash
#!/bin/bash
set -euo pipefail
chronyc tracking | grep -E "System time|Last offset"
systemctl restart chronyd
sleep 5
chronyc makestep
chronyc tracking | grep -E "System time|Last offset"
```

**S13：DNS 解析失败 → 重启 systemd-resolved + 验证**

- timeout: 60

```bash
#!/bin/bash
set -euo pipefail
echo "before: $(dig +short @127.0.0.53 example.com || true)"
systemctl restart systemd-resolved
sleep 2
echo "after:  $(dig +short @127.0.0.53 example.com)"
```

**S14：sysctl 参数漂移恢复**

- 触发：`node_net_somaxconn != 65535`
- timeout: 10

```bash
#!/bin/bash
set -euo pipefail
sysctl -w net.core.somaxconn=65535
grep -q "net.core.somaxconn=65535" /etc/sysctl.d/99-n9e.conf || \
    echo "net.core.somaxconn=65535" >> /etc/sysctl.d/99-n9e.conf
```

### 7.5 Windows 场景（PowerShell）

**S15：Windows 服务挂掉 → Restart-Service**

- 触发：`windows_service_state{state!="Running"} == 1`
- timeout: 120
- 在 task_tpl 里把 `account` 改为有权限的用户；脚本类型 `script_type: powershell`（categraf-win 支持）

```powershell
$payload = [Console]::In.ReadToEnd() | ConvertFrom-Json
$svc = $payload.service
if (-not $svc) { Write-Error "no service tag"; exit 2 }

Get-Service $svc | Format-Table
Restart-Service $svc -Force
Start-Sleep -Seconds 5
$st = (Get-Service $svc).Status
Write-Output "after restart: $st"
if ($st -ne "Running") { exit 1 }
```

**S16：Windows 磁盘清理（temp + Windows.old）**

- timeout: 300

```powershell
$before = (Get-PSDrive C).Free / 1GB
Get-ChildItem C:\Windows\Temp -Recurse -Force -ErrorAction SilentlyContinue |
    Where-Object { $_.LastWriteTime -lt (Get-Date).AddDays(-7) } |
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue
$after = (Get-PSDrive C).Free / 1GB
Write-Output ("freed {0:N2} GB" -f ($after - $before))
```

### 7.6 容器内场景

**S17：通过 `kubectl exec` 在 pod 内重启进程**

- 在中心机房一台带 kubeconfig 的"运维机"上注册 categraf，task_tpl 选这台执行
- timeout: 120

```bash
#!/bin/bash
set -euo pipefail
PAYLOAD=$(cat)
NS=$(echo "$PAYLOAD" | jq -r '.namespace // empty')
POD=$(echo "$PAYLOAD" | jq -r '.pod // empty')
CTN=$(echo "$PAYLOAD" | jq -r '.container // empty')

[ -z "$NS" ] || [ -z "$POD" ] || [ -z "$CTN" ] && { echo "need namespace/pod/container in labels"; exit 2; }

kubectl -n "$NS" exec "$POD" -c "$CTN" -- supervisorctl restart app || \
kubectl -n "$NS" delete pod "$POD"     # 兜底重建
```

**S18：通过 `docker exec` 在容器内执行**

- timeout: 60

```bash
#!/bin/bash
set -euo pipefail
PAYLOAD=$(cat)
CNAME=$(echo "$PAYLOAD" | jq -r '.container_name // empty')
[ -z "$CNAME" ] && { echo "no container_name"; exit 2; }
docker inspect "$CNAME" >/dev/null || { echo "no such container"; exit 2; }
docker exec "$CNAME" sh -c 'kill -HUP 1'    # 让 PID 1 自己 reload
```

### 7.7 中间件场景

**S19：MySQL 长事务 kill（带白名单 user）**

- 触发：`mysql_long_transaction_seconds > 600`
- timeout: 60

```bash
#!/bin/bash
set -euo pipefail
PAYLOAD=$(cat)
THRESH=600

# 只 kill 用户连接，绕开系统用户
mysql -N -e "
SELECT trx_mysql_thread_id
FROM information_schema.innodb_trx t
JOIN information_schema.processlist p ON p.ID=t.trx_mysql_thread_id
WHERE TIMESTAMPDIFF(SECOND, trx_started, NOW()) > $THRESH
  AND p.USER NOT IN ('root','repl','backup','event_scheduler')
" | while read tid; do
    [ -n "$tid" ] && mysql -e "KILL $tid"
done
```

**S20：Redis 内存超阈值 → 强制 BGREWRITEAOF + 提示扩容**

- timeout: 120

```bash
#!/bin/bash
set -euo pipefail
USED=$(redis-cli INFO memory | awk -F: '/^used_memory_rss:/{gsub(/\r/,""); print $2}')
echo "rss: $USED bytes"
redis-cli BGREWRITEAOF
echo "BGREWRITEAOF triggered, NOT auto-restarting redis; please扩容 maxmemory or scale out"
```

---

## 8. 调试与排错

### 8.1 "脚本一直 running"

按时间顺序对照 issue：

| 现象 | 根因 | 状态 |
|---|---|---|
| stdout 不刷新就一直 running | ibex < v8.3.0 的反馈不闭环 bug | #2841 修复，**升级到 v8.3.0+** |
| 脚本里有 `while true; sleep`（自己不退出） | 写法问题 | 加 timeout、加最大循环次数 |
| 任务超时但 categraf 没收到 kill | 网络抖动 | 等 task_host 表 status 自动转 failed（依赖心跳） |

### 8.2 "stdin 拿不到"

排查顺序：

1. **打开手动任务测试**：在"任务中心"手动建一次任务，stdin 输入 `{"ident":"host01"}`，看脚本能不能 echo 出来
2. **看 PromQL `by()` 子句**：自愈缺标签 90% 是这条；告警事件 detail 页里能看到 event.TagsJSON 真实有什么
3. **看脚本读法**：`read VAR` 只读一行；要用 `cat` 或 `python json.load(sys.stdin)` 一次读完

### 8.3 "exit code 0 但服务没真的恢复"

- 强制写 `set -e`（任何子命令失败立即退出）
- 关键步骤后加显式验证：`systemctl is-active "$SVC" || exit 1`
- 在 stdout 输出 before/after，让 task_record 可观察

### 8.4 查执行历史的 SQL（用户问"任务跑没跑成功"）

```sql
-- 看某个事件触发的所有自愈任务
SELECT id, title, create_at, FROM_UNIXTIME(create_at) AS ts
FROM task_record
WHERE event_id = <event_id>
ORDER BY id DESC;

-- task_host_x 是分片表（按 task_id mod 26）
-- 假设 task_id=12345, 12345%26=21
SELECT id, host, status, stdout, stderr
FROM task_host_21
WHERE id = 12345;
```

任务历史**设计为不可删除**（#2024-12-26 维护者明确"任务都是证据"）——别教用户删，告诉他们这是审计要求。

---

## 9. 输出风格

用户问"帮我写一个 X 的自愈脚本"时按这个套路：

1. **一句话点假设场景**（"假设触发条件是 `disk_used_percent > 90`，机器装了 jq"）
2. **fenced 脚本块**——必含 stdin 解析 + set -euo pipefail + before/after echo + 主体；对应灰名单命令必含护栏
3. **stdin 字段说明**：只列脚本里真用到的 key，标明哪些是 PromQL 标签、哪些是 ibex 注入
4. **运行参数建议**：`timeout=120 / batch=0 / tolerance=0 / pause=空 / account=root`，并给理由
5. **风险与回滚**：destructive 动作必给回滚方法或"不可逆"提示

用户问"为什么 X" 时：直接答，**不要丢一整段脚本**——尤其 is_recovered 类问题先纠概念再说替代方案。

用户要求 destructive 命令：陈述风险 + 给安全替代，绝不直接生成。
