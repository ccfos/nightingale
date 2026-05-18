# n9e-cli 设计决策

为 Cursor、Codex、Claude 等 AI Agent 提供高效操作夜莺告警规则、查看告警事件的命令行工具。

## 1. 方案选型：CLI 优先，MCP 为辅

### 1.1 为什么选 CLI

| 维度 | CLI | MCP |
|------|-----|-----|
| Token 成本 | 按需消耗，仅在调用时占用 token | 工具 schema 始终注入上下文，即使不用也消耗 token |
| Token 效率 | 比 MCP 节省 4-32 倍（业界实测） | schema 膨胀严重，15 个工具约 3000+ token |
| 可靠性 | 100%（stateless 二进制，无连接问题） | ~72%（TCP 超时、连接丢失等） |
| 渐进式披露 | Agent 按需运行 `--help` 发现命令 | 所有工具定义一次性加载到 system prompt |
| 可组合性 | 天然支持管道、jq、xargs | 封闭在 JSON-RPC 协议中 |

### 1.2 MCP 仍有价值的场景

- 无 shell 访问的环境（纯 API 调用的 Agent）
- 多租户认证场景
- 需要结构化审计日志
- 50+ 工具需要动态发现

### 1.3 混合策略

CLI 和 MCP Server（已有 [n9e-mcp-server](https://github.com/n9e/n9e-mcp-server)）共享底层 API 调用库，各自作为不同场景的入口。

### 1.4 三层颗粒度模型

API、CLI、MCP 服务于不同消费者，设计约束不同，颗粒度必须不同：

```
消费者          颗粒度    设计约束
──────────────────────────────────────────────────────────
MCP Tool       最粗     每个 tool schema 占 150-800 token，始终驻留上下文
                        → 工具数必须少（5-12 个），每个工具要"顶用"
──────────────────────────────────────────────────────────
CLI Command    中等     按需 --help 发现，仅调用时消耗 token
                        → 可以多一些命令（15-25 个），每个命令要自足
──────────────────────────────────────────────────────────
HTTP API       最细     前端/后端/CLI/MCP 全部依赖的基础层
                        → RESTful、一个资源一个端点、原子操作（30-50 个）
```

#### 同一场景下的颗粒度差异示例

场景：查看某业务组最近 1 小时严重级别的活跃告警，并对其中一条创建屏蔽规则。

**API 层（4 次调用，原子操作）**：

```
GET  /api/n9e/busi-groups/mine                                  → 获取业务组列表
GET  /api/n9e/alert-cur-events/list?gid=1&severity=1&hours=1   → 查活跃告警
GET  /api/n9e/alert-cur-event/42                                → 获取告警详情
POST /api/n9e/busi-group/1/alert-mutes                          → 创建屏蔽规则
```

**CLI 层（2 次调用，操作导向）**：

```bash
n9e-cli alert active list --group-id 1 --severity 1 --hours 1 --json
# --group-name infra 也可以，CLI 自动解析 name→ID

n9e-cli mute create --group-id 1 --json < mute.json
```

**MCP 层（1 次调用，任务导向）**：

```json
{
  "tool": "investigate_alerts",
  "params": {
    "group": "infrastructure",
    "severity": "critical",
    "hours": 1,
    "action": "list_and_summarize"
  }
}
```

#### 颗粒度设计对比

| 维度 | API | CLI | MCP |
|------|-----|-----|-----|
| 设计哲学 | 资源导向（Resource） | 操作导向（Operation） | 任务导向（Task） |
| 数量 | 多（30-50 端点） | 中等（15-25 命令） | 少（5-12 工具） |
| 一次调用完成 | 一个原子操作 | 一个用户意图 | 一个完整场景 |
| ID 解析 | 调用者负责 | 支持 name→ID 自动解析 | 内部自动解析 |
| 聚合 | 无 | 轻度（常用组合） | 重度（多步编排） |
| 输出控制 | 固定 JSON schema | `--json`/`--quiet`/flags 灵活控制 | 返回 Agent 可直接推理的摘要 |
| 分页 | `limit`+`offset` 参数 | `--limit` flag，默认合理值 | 内部自动处理，返回 top-N |
| Token 成本 | 不直接涉及 | 按需（仅调用时） | schema 常驻（150-800 token/tool） |

#### 各层颗粒度设计原则

**API（最细粒度，保持现状）**：

夜莺已有的 API 颗粒度是正确的——RESTful、原子操作。这是基础层，不需要为 CLI/MCP 改动。

**CLI（中粒度，~20 条命令）**：

- 大部分命令与 API 1:1 对应，保持简单可预测
- 少量高频场景做聚合（name→ID 解析、批量操作、import/export）
- 通过 flags 组合覆盖变体，而非新增命令

```bash
# 与 API 1:1 对应（Agent 需要精确操作时）
n9e-cli alert rule get --id 42 --json

# 比 API 更"聪明"（聚合 + 便捷参数）
n9e-cli alert active list --group-name infra --severity critical --json

# 复合命令（一个命令 = 多个 API 调用）
n9e-cli alert rule import --file rules.yaml --group-name infra --dry-run --json
```

**MCP（最粗粒度，~8 个工具）**：

- 每个 tool schema 占 150-800 token，工具数超过 20 后 Agent 选择准确率明显下降
- 用 `action` 参数在一个 tool 内区分 CRUD，而非拆成 4 个 tool
- 返回值做语义裁剪——返回 Agent 可直接推理的摘要，而非原始 JSON
- 业界推荐 5-12 个工具/server

```
# 推荐（任务导向，~8 个工具）：
query_alerts        → 统一查询活跃/历史告警，内置过滤和摘要
manage_alert_rules  → 列出/查看/创建/更新规则（action 参数区分）
manage_mutes        → 屏蔽规则的完整 CRUD
query_targets       → 查询监控目标及其状态
search              → 跨资源搜索（Agent 不确定要找什么时用）
get_overview        → 系统概览（告警统计、严重级别分布等）
```

#### 三层架构关系

```
┌──────────────────────────────────────────────────────┐
│  MCP Tool（任务层）                                    │
│  ~8 个粗粒度工具，面向 AI 对话场景                       │
│  每个工具内部编排多个 CLI/API 调用                       │
├──────────────────────────────────────────────────────┤
│  CLI Command（操作层）                                 │
│  ~20 个中粒度命令，面向 shell 场景                      │
│  每个命令可能聚合 1-3 个 API 调用                       │
├──────────────────────────────────────────────────────┤
│  HTTP API（资源层）                                    │
│  ~40 个细粒度端点，面向所有客户端                        │
│  原子操作，RESTful                                     │
└──────────────────────────────────────────────────────┘
```

CLI 和 MCP 都调用同一套 HTTP API，它们是 API 之上的门面层（Facade），针对各自消费者做不同程度的聚合。不直接操作数据库。

## 2. 设计原则

### 2.1 分层子命令 + 名词-动词层级

采用 `noun verb` 模式（对标 `docker container ls`、`gh pr create`），Agent 通过 `--help` 进行树状发现：

```
n9e-cli --help          → 看到所有资源（名词）
n9e-cli alert --help    → 看到 alert 下的子资源
n9e-cli alert rule --help → 看到 rule 的所有操作（动词）
```

### 2.2 结构化输出

- 所有命令支持 `--json` 输出
- JSON 输出到 stdout，日志/进度到 stderr
- 字段扁平化，避免深层嵌套
- 类型一致：时间戳统一用 Unix epoch 或 ISO 8601
- 流式输出使用 NDJSON（每行一个 JSON 对象）

### 2.3 退出码即控制流

```
0 = 成功
1 = 通用错误
2 = 参数错误（用法不对）
3 = 资源未找到
4 = 权限不足
5 = 冲突（资源已存在）
```

结合结构化错误输出：

```json
{"error": "not_found", "message": "alert rule 42 not found", "suggestion": "run n9e-cli alert rule list --group-id 1"}
```

### 2.4 Help 文本即文档

每个命令的 `--help` 必须包含：

- 明确标注 required / optional 参数
- 至少 2 个真实示例
- 提及 `--json` 标志
- 简洁的一句话描述

```
n9e-cli alert rule list --help
List alert rules for a business group.

Usage:
  n9e-cli alert rule list [flags]

Flags:
  --group-id int     Business group ID (required)
  --disabled         Only show disabled rules
  --json             Output as JSON
  --limit int        Max results (default: 20)
  --offset int       Pagination offset

Examples:
  n9e-cli alert rule list --group-id 1 --json
  n9e-cli alert rule list --group-id 1 --disabled --limit 10 --json
```

### 2.5 AI Agent 友好特性

- `--dry-run`：预览变更，输出结构化 diff
- `--yes` / `--force`：跳过确认提示（Agent 无法交互输入）
- `--quiet`：仅输出裸值，适合管道
- `--limit` / `--offset`：分页，避免一次返回所有记录
- 幂等操作：`create --if-not-exists`，或用 `apply` 语义
- 非交互终端自动检测：stdin 非 TTY 时跳过确认或报错

### 2.6 可组合性

```bash
# Agent 天然会组合这些命令
n9e-cli alert active list --json | jq '.[] | select(.severity == 1)'

# 内置过滤更省 token
n9e-cli alert active list --severity 1 --json

# 批量操作减少调用次数
n9e-cli alert rule delete --ids 1,2,3 --yes
```

### 2.7 可操作的错误信息

错误信息必须包含：
- 错误码/类型（可解析的字符串如 `"image_not_found"`）
- 失败的输入（回显参数）
- 建议的下一步操作
- 区分暂时性错误和永久性错误

### 2.8 资源标识符策略

#### 现状分析

夜莺所有核心模型均使用 MySQL `int64` 自增主键，可读标识字段情况参差不齐：

| 模型 | 主键 | 有 Name？ | Name 唯一性 |
|------|------|-----------|-------------|
| AlertRule | `int64` 自增 | 有 | `(group_id, name)` 应用层唯一，无 DB 约束 |
| AlertCurEvent | `int64`（取自 HisEvent.Id） | 无（有 RuleName 快照） | — |
| AlertHisEvent | `int64` 自增 | 无（有 RuleName 快照） | — |
| Dashboard | `int64` 自增 | 有 | `(group_id, name)` DB UNIQUE |
| AlertMute | `int64` 自增 | **无**（仅有 Note） | — |
| AlertSubscribe | `int64` 自增 | 有 | 无唯一约束 |
| EventPipeline | `int64` 自增 | 有 | 无唯一约束 |
| BusiGroup | `int64` 自增 | 有 | **全局 DB UNIQUE** |

另外，AlertCurEvent / AlertHisEvent 有 `hash` 字段（`rule_id + vector_key`），可用于定位同规则同标签组合的事件。

#### 设计决策：不改 DB schema，CLI 层做智能解析

**不推荐给所有表加 UUID / slug 列**，理由：
- 夜莺是成熟项目，线上用户多，schema 变更有 migration 成本
- Name 在业务组内不唯一是合理设计（不同团队可以有同名规则）
- 前端、API、MCP Server 都要适配

**推荐 CLI 层做"聪明的翻译层"**，参考 `gh`（GitHub CLI）和 `kubectl` 的做法：

#### 参数接受规则

所有 get/update/delete 命令同时支持 `--id` 和 `--name`：

```bash
# 精确：按 ID（给 Agent 和脚本用）
n9e-cli alert rule get --id 42

# 友好：按名字（给人和 Agent 用）
n9e-cli alert rule get --name "CPU使用率告警"

# 消歧：名字 + 业务组范围
n9e-cli alert rule get --name "CPU告警" --group-name "基础设施"
```

#### 业务组名字自动解析

所有需要 `group_id` 的命令同时支持 `--group-id` 和 `--group-name`：

```bash
# 传统方式：需先查 group ID
n9e-cli alert rule list --group-id 1 --json

# CLI 自动解析：--group-name → 内部查 BusiGroup → 拿到 ID
n9e-cli alert rule list --group-name "基础设施" --json
```

BusiGroup.Name 是全局 DB UNIQUE，可以安全地做 name→ID 解析。

#### 名字匹配逻辑

```
解析优先级：--id > --name
匹配规则：
  精确匹配 1 条  → 返回结果
  匹配多条      → 返回列表 + 提示用 --id 或加 --group-name 消歧
  无匹配        → exit code 3 + suggestion 提示用 list 命令
```

#### 告警事件的标识

AlertCurEvent 已有 `hash` 字段，CLI 同时支持：

```bash
n9e-cli alert active get --id 12345        # 按事件 ID
n9e-cli alert active get --hash "abc123"   # 按 hash（同规则同标签的事件 hash 一致）
```

#### 无 Name 的模型（AlertMute）

AlertMute 只有 `note` 字段，不适合做标识。CLI 策略：

- list 命令做好过滤（`--group-name`、`--active`、`--severity`）
- Agent 典型流程是先 list 再从输出中提取 ID 做后续操作
- CLI 输出始终包含 `id` 字段，便于 Agent 提取

#### 输出中始终包含 ID

所有 list/get 命令的 JSON 输出必须包含 `id` 字段在最前面，Agent 可从 list 结果提取 ID 用于后续的 get/update/delete 操作：

```json
[
  {"id": 42, "name": "CPU使用率告警", "group_id": 1, "severity": 1, "disabled": 0},
  {"id": 43, "name": "内存使用率告警", "group_id": 1, "severity": 2, "disabled": 0}
]
```

## 3. 命令树

```
n9e-cli
├── alert
│   ├── rule
│   │   ├── list          # 列出告警规则
│   │   ├── get           # 获取规则详情
│   │   ├── create        # 创建规则
│   │   ├── update        # 更新规则
│   │   └── delete        # 删除规则
│   ├── active
│   │   ├── list          # 列出活跃告警事件
│   │   └── get           # 获取活跃告警详情
│   └── history
│       ├── list          # 历史告警查询
│       └── get           # 历史告警详情
├── mute
│   ├── list              # 列出屏蔽规则
│   ├── get               # 屏蔽规则详情
│   ├── create            # 创建屏蔽规则
│   ├── update            # 更新屏蔽规则
│   └── delete            # 删除屏蔽规则
├── subscribe
│   ├── list              # 列出告警订阅
│   ├── get               # 订阅详情
│   ├── create            # 创建订阅
│   └── update            # 更新订阅
├── notify
│   ├── rule
│   │   ├── list          # 列出通知规则
│   │   └── get           # 通知规则详情
│   └── channel
│       └── list          # 列出通知渠道
├── target
│   ├── list              # 列出监控目标
│   └── get               # 监控目标详情
├── datasource
│   └── list              # 列出数据源
├── event-pipeline
│   ├── list              # 列出事件管道
│   ├── get               # 管道详情
│   └── runs              # 查看执行记录
└── busi-group
    └── list              # 列出业务组
```

## 4. 对应的夜莺 API 路由

CLI 底层调用夜莺已有的 HTTP API：

| CLI 命令 | 夜莺 API 路由 | Handler 文件 |
|----------|---------------|--------------|
| `alert rule list` | `GET /api/n9e/busi-group/:id/alert-rules` | `router_alert_rule.go` |
| `alert rule get` | `GET /api/n9e/alert-rule/:arid` | `router_alert_rule.go` |
| `alert active list` | `GET /api/n9e/alert-cur-events/list` | `router_alert_cur_event.go` |
| `alert active get` | `GET /api/n9e/alert-cur-event/:eid` | `router_alert_cur_event.go` |
| `alert history list` | `GET /api/n9e/alert-his-events/list` | `router_alert_his_event.go` |
| `alert history get` | `GET /api/n9e/alert-his-event/:eid` | `router_alert_his_event.go` |
| `mute list` | `GET /api/n9e/busi-group/:id/alert-mutes` | `router_mute.go` |
| `subscribe list` | `GET /api/n9e/busi-group/:id/alert-subscribes` | `router_alert_subscribe.go` |
| `notify rule list` | `GET /api/n9e/busi-group/:id/notify-rules` | `router_notify_rule.go` |
| `target list` | `GET /api/n9e/targets` | `router_target.go` |
| `datasource list` | `GET /api/n9e/datasource/list` | `router_datasource.go` |
| `busi-group list` | `GET /api/n9e/busi-groups/mine` | `router_busi_group.go` |

Handler 文件位于 `center/router/` 目录下。

## 5. Token 优化策略

1. **CLI 按需披露**：Agent 只在需要时调用 `--help`（~200-500 token），不像 MCP 预加载所有 schema（~3000+ token/次）
2. **AGENTS.md 指引文件**：在项目根目录放置约 800 token 的 cookbook，描述核心用法。人写的说明文件比 LLM 生成的效果更好，推理成本低 ~20%
3. **扁平 JSON**：减少解析 token
4. **内置分页**：`--limit` 默认 20 条，避免返回海量数据
5. **内置过滤**：`--severity`、`--status` 等参数，避免 Agent 用 jq 二次过滤

## 6. 技术选型

- **框架**：[Cobra](https://github.com/spf13/cobra)（kubectl、gh、docker 均使用）
- **配置**：[Viper](https://github.com/spf13/viper)（API 地址、Token 等）
- **认证**：支持环境变量 `N9E_API_URL`、`N9E_TOKEN` 和配置文件 `~/.n9e-cli.yaml`
- **语言**：Go（与夜莺主项目一致，可复用类型定义）

## 7. 配置管理

```yaml
# ~/.n9e-cli.yaml
api_url: http://localhost:17000
token: your-api-token
default_output: json
```

也支持环境变量覆盖：

```bash
export N9E_API_URL=http://localhost:17000
export N9E_TOKEN=your-api-token
```

优先级：命令行参数 > 环境变量 > 配置文件

## 8. 参考资料

- [Writing CLI Tools That AI Agents Actually Want to Use](https://dev.to/uenyioha/writing-cli-tools-that-ai-agents-actually-want-to-use-39no)
- [MCP vs CLI: Benchmarking AI Agent Cost & Reliability](https://www.scalekit.com/blog/mcp-vs-cli-use)
- [MCP vs CLI for AI Agents: I Measured the Same Tool Both Ways](https://afrozeamjad.com/writing/mcp-vs-cli-token-benchmark/)
- [Building CLI for Agents](https://docs.hiroleague.com/ai-coding-bible/building-cli-for-agents)
- [Your MCP server is a monolith. Here's how to fix it](https://www.channel.tel/blog/mcp-server-monolith-fix-tool-scoping) — MCP 工具颗粒度与 5-12 工具/server 原则
- [How MCP Tool Definitions Inflate Your AI Agent Token Costs](https://docs.bswen.com/blog/2026-04-24-mcp-token-overhead/) — 单工具 schema 150-800 token 的实测数据
- [MCP Token Optimization: 4 Approaches Compared](https://stackone.com/blog/mcp-token-optimization/)
- [How Granular Should You Design APIs?](https://nordicapis.com/how-granular-should-you-design-apis/) — API 颗粒度设计原则
- [Levels of API granularity](https://world.hey.com/boriseetgerink/levels-of-api-granularity-48e0967d) — Resource / Aggregate / Facade 三层模型
- [n9e-mcp-server](https://github.com/n9e/n9e-mcp-server)
