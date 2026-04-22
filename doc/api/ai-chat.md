# AI Chat API

AI 对话助手接口，支持多轮对话、流式输出、消息取消等功能。

提供两套路由，共享相同的核心逻辑：

| 路由前缀 | 认证方式 | 用户身份 | 使用场景 |
|----------|---------|---------|---------|
| `/api/n9e` | JWT / Token / ProxyAuth | 从认证信息自动获取 | 前端页面调用 |
| `/v1/n9e` | BasicAuth（`APIForService`） | 请求体中的 `username` 字段 | 外部服务调用 |

---

## 前后端交互概览

### 整体架构

前端 AICopilot 组件以侧边抽屉形式嵌入各业务页面（指标查询 Explorer、仪表盘、告警规则等），通过以下接口与后端协作完成 AI 对话：

```
┌─────────────────────────────────────────────────────────────────┐
│  前端 (AICopilot)                                                │
│                                                                   │
│  CopilotEntry / CopilotButton                                    │
│       │                                                           │
│       ▼                                                           │
│  ┌─────────────┐    ┌───────────────┐    ┌────────────────────┐  │
│  │ Conversation │    │   ChatPanel   │    │    useStream       │  │
│  │   Header     │    │  (核心逻辑)    │    │  (SSE 流式消费)    │  │
│  └──────┬───────┘    └───────┬───────┘    └────────┬───────────┘  │
│         │                    │                      │              │
└─────────┼────────────────────┼──────────────────────┼─────────────┘
          │                    │                      │
          ▼                    ▼                      ▼
   GET  /chat/history   POST /message/new      POST /stream (SSE)
   POST /chat/new       POST /message/detail
   DELETE /chat/:id     POST /message/history
                        POST /message/cancel
          │                    │                      │
          ▼                    ▼                      ▼
┌─────────────────────────────────────────────────────────────────┐
│  后端 (router_ai_assistant.go)                                   │
│                                                                   │
│  会话管理 (Redis)  ──►  消息处理 (异步)  ──►  AI Agent (LLM)    │
│                          │                                        │
│                          ▼                                        │
│                    StreamCache (SSE 推送)                         │
└─────────────────────────────────────────────────────────────────┘
```

### 核心交互时序

```
前端                                后端                          AI Agent
 │                                   │                              │
 │  1. POST /chat/new                │                              │
 │  (携带 page 和 datasource 上下文)  │                              │
 │──────────────────────────────────►│                              │
 │◄──── 返回 chat_id ───────────────│                              │
 │                                   │                              │
 │  2. POST /message/new             │                              │
 │  (chat_id + 用户问题)             │                              │
 │──────────────────────────────────►│  分配 seq_id, 创建 stream_id │
 │◄──── 返回 {chat_id, seq_id} ─────│                              │
 │                                   │  3. 异步调用 AI Agent ───────►│
 │  4. POST /message/detail          │                              │
 │  (获取 stream_id)                 │                              │
 │──────────────────────────────────►│                              │
 │◄──── 返回消息详情(含 stream_id) ──│                              │
 │                                   │                              │
 │  5. POST /stream                  │                              │
 │  (用 stream_id 建立 SSE 连接)      │                              │
 │──────────────────────────────────►│                              │
 │                                   │◄──── 流式返回 token ─────────│
 │◄──── SSE: data {delta} ──────────│                              │
 │◄──── SSE: data {delta} ──────────│                              │
 │◄──── SSE: data {delta} ──────────│                              │
 │                                   │◄──── 处理完成 ───────────────│
 │◄──── SSE: event: finish ─────────│                              │
 │                                   │                              │
 │  6. POST /message/detail          │                              │
 │  (获取最终完整回复)                │                              │
 │──────────────────────────────────►│                              │
 │◄──── 返回完整消息 ───────────────│                              │
```

### 前端关键实现说明

#### 消息发送与回复获取

1. **发送消息**：用户输入后调用 `POST /message/new`，立即获得 `seq_id`。前端在消息列表中插入一条占位消息并显示加载状态。

2. **轮询 + 流式双通道**：
   - 前端每 **3 秒**轮询 `POST /message/detail` 获取消息状态（`is_finish`、`cur_step`、`err_msg` 等）
   - 同时从首次轮询结果中取出 `response[0].stream_id`，建立 SSE 连接实时接收文本增量
   - 轮询负责获取结构化状态，SSE 负责实时文本渲染，两者互补

3. **SSE 流式解析**：前端使用 `fetch` + `ReadableStream` 消费 SSE（非 EventSource），解析每行 `data:` JSON：
   - `{v: "增量文本", p: "content"}` — AI 回复内容，拼接后实时渲染
   - `{v: "增量文本", p: "reason"}` — AI 思考过程，展示在可折叠的 ThinkingBlock 中
   - `event: finish` — 流结束信号

   > **思考过程持久化**：AI 的思考过程（thinking/reasoning）在流式结束后会作为 `content_type: "reasoning"` 的 response 元素持久化到数据库。加载历史对话时，前端从 `response` 数组中读取 `reasoning` 元素展示 ThinkingBlock，无需依赖 SSE 流。

4. **取消处理**：用户点击"停止"按钮时，同时执行：
   - `AbortController.abort()` 中断 SSE 连接
   - `POST /message/cancel` 通知后端取消 AI 处理

#### 上下文传递

前端根据所在页面自动携带上下文信息：

| 嵌入页面 | page 值 | 携带参数 |
|---------|---------|---------|
| 指标查询 Explorer | `explorer` | datasource_type, datasource_id |
| 仪表盘 | `dashboards` | datasource_type, datasource_id |
| 活跃告警 | `active_alert` | - |
| 历史告警 | `alert_history` | - |

支持的 `datasource_type`：`prometheus`、`mysql`、`doris`、`ck`（ClickHouse）、`pgsql`

#### 回复渲染

后端已将 AI 回复拆分为结构化的 `response` 数组，前端根据 `content_type` 做不同渲染：
- `reasoning`：渲染为可折叠的 ThinkingBlock，展示 AI 思考过程。实时流式时从 SSE `p:"reason"` 事件获取，加载历史消息时从 `response` 数组读取
- `query`：渲染为带语法高亮的 QueryResultBlock，附带「复制」和「执行查询」按钮
- `markdown`：直接按 Markdown 渲染文字说明
- `hint`：渲染为提示样式
- `alert_rule`：渲染为 AlertRuleResultBlock 卡片，展示创建成功的告警规则字段（规则 ID/名称/业务组/数据源/PromQL/阈值等）。规则名称是可点击的链接，新页面打开 `/alert-rules/edit/:id`
- `form_select`：渲染为 CreationFormSelector，渐进式展示创建类操作所需的字段（业务组 / 数据源 / 团队等），用户填完一个字段后下一个解锁，全部填完点「确定」自动重发上一条用户消息（详见下方 [创建类操作的 preflight 流程](#创建类操作的-preflight-流程)）

#### 前端关键文件

| 文件 | 作用 |
|------|------|
| `src/components/AICopilot/services.ts` | 所有 API 调用封装 |
| `src/components/AICopilot/useStream.ts` | SSE 流式消费 Hook |
| `src/components/AICopilot/ChatPanel.tsx` | 核心聊天逻辑与 UI 编排 |
| `src/components/AICopilot/types.ts` | TypeScript 类型定义 |
| `src/components/AICopilot/ConversationHeader.tsx` | 会话切换与管理 |
| `src/components/AICopilot/ThinkingBlock.tsx` | AI 思考过程展示 |
| `src/components/AICopilot/QueryResultBlock.tsx` | 查询结果提取与运行 |
| `src/components/AICopilot/AlertRuleResultBlock.tsx` | 告警规则创建成功后的卡片 |
| `src/components/AICopilot/CreationFormSelector.tsx` | 创建类操作的 preflight 表单 |
| `src/components/AICopilot/CopilotEntry.tsx` | 入口组件（抽屉模式） |

---

## 数据结构

### AssistantChat

| 字段 | 类型 | 说明 |
|------|------|------|
| chat_id | string | 会话 ID（UUID） |
| title | string | 会话标题，首条消息时自动截取用户输入前 50 字符 |
| last_update | int64 | 最后更新时间（Unix 时间戳） |
| page_from | AssistantPageInfo | 发起对话的页面来源 |
| user_id | int64 | 所属用户 ID |
| is_new | bool | 是否为新会话（尚未发送消息） |

### AssistantPageInfo

记录用户从哪个页面打开 AI 助手。`page` 用于前端展示对应页面的「快捷提问」按钮（前端硬编码，见 [ai-chat-recommend-action.md](./ai-chat-recommend-action.md)），`param` 为页面级别的上下文信息，不同页面携带的字段不同，部分页面可能不需要传。

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| page | string | 否 | 页面类型，枚举值见下表 |
| param | object | 否 | 页面级上下文，结构因页面而异，不需要时可省略 |

#### page 枚举值

| page | 来源页面 | param 建议字段 |
|------|----------|----------------|
| `explorer` | 时序数据探索 | `datasource_type`, `datasource_id` |
| `dashboards` | 仪表盘列表 | `busi_group_id`（可选） |
| `alert_rule` | 告警规则列表 | `busi_group_id`（可选） |
| `alert_history` | 历史告警列表 | 可省略 |
| `active_alert` | 活跃告警列表 | 可省略 |
| `notify_tpl` | 消息模板配置 | 可省略 |
| `datasource` | 数据源配置 | `datasource_type`, `datasource_id`（可选） |

**param 示例（explorer 页面）：**

```json
{
  "datasource_type": "prometheus",
  "datasource_id": 1
}
```

### AssistantMessage

| 字段 | 类型 | 说明 |
|------|------|------|
| chat_id | string | 所属会话 ID |
| seq_id | int64 | 消息序号，从 1 开始自增 |
| query | AssistantMessageQuery | 用户提问 |
| response | AssistantMessageResponse[] | AI 回复（数组） |
| cur_step | string | 当前处理步骤描述（如 "Using tools..."），处理完成后为空 |
| is_finish | bool | 消息是否处理完成 |
| recommend_action | AssistantAction[] | 推荐的后续操作 |
| err_code | int | 错误码，0 表示成功，-2 表示已取消，其他为异常 |
| err_title | string | 错误标题 |
| err_msg | string | 错误信息 |
| executed_tools | bool | 是否执行了工具调用 |

### AssistantMessageQuery

| 字段 | 类型 | 说明 |
|------|------|------|
| content | string | 用户输入内容（必填） |
| action | AssistantAction | 指定的操作（可选，不指定时根据 page_from 自动推断） |
| page_from | AssistantPageInfo | 发起消息的页面来源 |

### AssistantMessageResponse

| 字段 | 类型 | 说明 |
|------|------|------|
| content_type | string | 内容类型：`reasoning` / `markdown` / `query` / `hint` / `alert_rule` / `form_select`（见下方说明） |
| content | string | 回复内容（处理完成后填充） |
| stream_id | string | 流式 ID，用于订阅 SSE 流（仅第一个元素携带） |
| is_finish | bool | 该条回复是否已完成 |
| is_from_ai | bool | 是否来自 AI |

**content_type 说明：**

| 值 | 说明 | content 格式 |
|------|------|------|
| `reasoning` | AI 思考过程 | 纯文本，前端渲染为可折叠的 ThinkingBlock |
| `markdown` | 文字说明 | Markdown 文本，直接渲染 |
| `query` | 查询语句（PromQL / SQL） | 纯查询语句字符串，前端渲染为代码块 + 复制/执行按钮 |
| `hint` | 提示信息 | 提示文本 |
| `alert_rule` | 告警规则创建成功的结构化结果 | JSON 字符串，字段见下方「[`alert_rule` content 结构](#alert_rule-content-结构)」 |
| `form_select` | preflight 缺少必要上下文，需要用户在表单里补充后再继续 | JSON 字符串，字段见下方「[`form_select` content 结构](#form_select-content-结构)」 |

**`reasoning` 元素**：如果 AI 在处理过程中产生了思考过程（thinking/reasoning），会作为 `response` 数组的**第一个元素**持久化保存。这样加载历史对话时，前端仍然可以展示完整的思考过程。实时流式传输时，思考过程通过 SSE 的 `p: "reason"` 事件推送；加载历史消息时，从 `response` 数组中 `content_type === "reasoning"` 的元素读取。

对于 `query_generator` 类型的操作，AI 处理完成后 `response` 数组最多包含三个元素：
1. **`reasoning` 元素**（可选）：AI 的思考过程
2. **`query` 元素**：纯查询语句（如 `cpu_usage_active{cpu="cpu-total"}`），前端可直接用于执行查询
3. **`markdown` 元素**：查询的文字说明（如"使用 cpu-usage-active 指标查询主机 CPU 使用率..."）

如果 AI 返回的内容无法解析为结构化的 query+explanation JSON，则回退为单个 `markdown` 元素（可能带 `reasoning` 前缀）。

对于 `creation` 类型的操作（创建告警规则 / 仪表盘 / 屏蔽规则等），完成后的 `response` 数组通常包含：
1. **`reasoning` 元素**（可选）：AI 的思考过程
2. **`markdown` 元素**：一句话的简短确认（如 "✅ 已为您创建告警规则「主机 CPU 使用率过高」"）
3. **`alert_rule` 元素**（创建告警规则时）：结构化卡片数据。每成功调用一次 `create_alert_rule` 工具就追加一个

如果在 agent 跑起来之前 preflight 检测到缺必填上下文（业务组 / 数据源 / 团队），`response` 数组只会包含一个 `form_select` 元素，agent 不会执行。详见 [创建类操作的 preflight 流程](#创建类操作的-preflight-流程)。

#### `alert_rule` content 结构

`content` 字段是 JSON 字符串，反序列化后字段如下：

| 字段 | 类型 | 说明 |
|------|------|------|
| id | int64 | 告警规则 ID |
| name | string | 规则名称（前端渲染为可点击链接，跳 `/alert-rules/edit/:id`） |
| group_id | int64 | 业务组 ID |
| group_name | string | 业务组名称 |
| datasource_id | int64 | 数据源 ID（host 类型规则可能为 0） |
| datasource_name | string | 数据源名称 |
| cate | string | 数据源类型（prometheus/mysql/...） |
| prod | string | 产品类型（metric/logging/host） |
| severity | int | 告警级别：1=Critical, 2=Warning, 3=Info |
| prom_ql | string | PromQL（仅 prometheus 简化路径有） |
| operator | string | 比较操作符（仅 prometheus 简化路径有） |
| threshold | number | 阈值（仅 prometheus 简化路径有） |
| for_duration | int | 持续时长秒数 |
| note | string | 告警说明 |

#### `form_select` content 结构

`content` 字段是 JSON 字符串，反序列化后字段如下：

| 字段 | 类型 | 说明 |
|------|------|------|
| skill_name | string | 触发本次 preflight 的 skill（如 `n9e-create-alert-rule`），前端可用于差异化展示 |
| fields | array | 需要用户补充的字段列表，按顺序渲染 |

每个 field 的结构：

| 字段 | 类型 | 说明 |
|------|------|------|
| key | string | 字段标识，目前支持 `busi_group_id` / `datasource_id` / `team_ids`。提交时按这个 key 写到 `action.param` 对应字段 |
| type | string | `single`（单选） / `multi`（多选） |
| candidates | array | 候选项列表，已按用户权限过滤好 |

每个 candidate 的结构：

| 字段 | 类型 | 说明 |
|------|------|------|
| id | int64 | 候选项 ID |
| name | string | 候选项名称 |
| is_default | bool | 前端应预选此项（busi_group 走名称启发式，其他字段优先匹配请求里已有的值） |
| extra | string | 附加展示信息（如 datasource 的 `plugin_type`），可选 |

### AssistantAction

| 字段 | 类型 | 说明 |
|------|------|------|
| content | string | 操作展示文案 |
| key | string | 操作标识，枚举值见下表 |
| param | AssistantActionParam | 操作参数 |

#### key 枚举值

| key | 用途 |
|-----|------|
| `query_generator` | 生成 PromQL / SQL / 日志查询 |
| `general_chat` | 通用问答（LLM 意图分类兜底） |
| `alert_query` | 查询告警事件（活跃 / 历史） |
| `resource_query` | 查询监控资源（告警规则、机器、仪表盘、屏蔽、订阅、通知规则、数据源、用户、团队、业务组） |
| `creation` | 创建资源（告警规则 / 仪表盘 / 屏蔽 / 订阅 / 通知规则），可能触发 preflight |
| `troubleshooting` | 故障排查 / 根因分析 |
| `notify_template_generator` | 生成/修改告警通知消息模板（钉钉、飞书、邮件等） |
| `datasource_diagnose` | 数据源连通性 / 配置诊断（x509、401、URL 格式等） |

### AssistantActionParam

| 字段 | 类型 | 说明 |
|------|------|------|
| datasource_type | string | 数据源类型（可选） |
| datasource_id | int64 | 数据源 ID（可选）。从查询编辑器入口打开时由 actionContext 自动注入；用户在 `form_select` 表单里手选后会覆盖 |
| database_name | string | 数据库名（可选） |
| table_name | string | 表名（可选） |
| busi_group_id | int64 | 业务组 ID（可选）。用户在 `form_select` 表单里选择业务组后由前端自动写入；后续同会话消息也会带上 |
| team_ids | int64[] | 通知团队 ID 列表（可选）。用户在 `form_select` 表单里选择团队（多选）后由前端自动写入；通知规则创建场景需要 |

---

## 创建类操作的 preflight 流程

创建类操作（创建告警规则 / 仪表盘 / 屏蔽 / 订阅 / 通知规则）需要的上下文（业务组、数据源、团队）通常没法从用户的自然语言里完整地解析出来。后端不让 LLM 自己猜测，而是在 agent 跑起来**之前**用一个 preflight 钩子检测：缺啥就让前端弹个表单让用户填，填完再继续。

### 触发条件

后端按下面顺序识别"用户要创建什么"：

1. 当前用户消息命中 `creation` 关键词 fast-path（比如包含"创建/新建"+"告警规则/仪表盘/屏蔽规则/订阅/通知规则"），路由到 `creation` action_key，**绕过 LLM 意图分类器**
2. 否则，第一条消息走前端传的 `action.key`，后续消息走 LLM 意图分类
3. `creation` action 的 preflight 把命中的 skill 所需的上下文字段（按 skill 来）和当前请求里已有的字段做对比，**任一字段缺失**就触发 halt

每个 skill 的依赖：

| skill | 必填上下文 |
|------|----------|
| n9e-create-alert-rule | busi_group_id, datasource_id |
| n9e-create-dashboard | busi_group_id |
| n9e-create-alert-mute | busi_group_id |
| n9e-create-alert-subscribe | busi_group_id |
| n9e-create-notify-rule | team_ids |

> 仪表盘不强制前置数据源：一个仪表盘可以有跨数据源的面板，且 page_from / `list_datasources` 能兜底，所以 preflight 不在这里"锁死"数据源。

### 两轮交互时序

```
前端                                    后端
 │                                       │
 │  POST /message/new                    │
 │  query.content="创建一条 host disk 告警规则"
 │  action.param.datasource_id=1（来自页面上下文）│
 │──────────────────────────────────────►│
 │                                       │  ① 识别为 creation
 │                                       │  ② preflight 检测：
 │                                       │     - busi_group_id 缺失
 │                                       │     - datasource_id 已有 → 仅作默认
 │                                       │     → 任一字段缺 → 弹 form
 │                                       │  ③ 加载候选（busi_groups + datasources）
 │                                       │  ④ 不跑 agent，直接 finish
 │◄──── response: [form_select] ─────────│
 │                                       │
 │  ⑤ 渲染 CreationFormSelector          │
 │  ⑥ 用户选业务组、确认数据源、点「确定」 │
 │  ⑦ 隐藏 halted turn，自动重发上一条用户消息  │
 │                                       │
 │  POST /message/new                    │
 │  query.content="创建一条 host disk 告警规则"（同上一条）│
 │  action.param.busi_group_id=1（用户填的）   │
 │  action.param.datasource_id=1（用户确认的） │
 │──────────────────────────────────────►│
 │                                       │  ⑧ 仍是 creation
 │                                       │  ⑨ preflight 全部满足 → 不 halt
 │                                       │  ⑩ agent 跑 ReAct → create_alert_rule
 │◄──── response: [reasoning, markdown, alert_rule] ──│
 │                                       │
 │  ⑪ 渲染 ThinkingBlock + 一句话确认 + AlertRuleResultBlock 卡片 │
```

### 前端要做的事

**会话级状态**：在 `ChatPanel` 里维护一个 `sessionParam: { busi_group_id?, datasource_id?, team_ids? }`，切换 chat 时清空。

**发送消息时**：把 `sessionParam` merge 到 `action.param` 里。已有用户选择的字段优先于页面上下文字段：

```ts
action.param = {
  datasource_type: actionContext?.datasource_type,
  datasource_id: sessionParam.datasource_id ?? actionContext?.datasource_id,
  ...(sessionParam.busi_group_id && { busi_group_id: sessionParam.busi_group_id }),
  ...(sessionParam.team_ids?.length && { team_ids: sessionParam.team_ids }),
};
```

**收到 `form_select` 时**：渲染 `CreationFormSelector` 组件。组件按 `fields` 顺序渐进式展示，每个字段选完才解锁下一个；`is_default=true` 的候选预选。点「确定」时：

1. 把所有选中的 id 写到 `sessionParam`
2. 从可见消息列表里**移除**当前 halted turn（用户已经填完了，没必要继续展示空表单）
3. 调 `handleSend(原始 user content, 选中的 values)` 自动重发，`action.param` 带上新值

**halted message 持久化**：halted message 后端会保存，刷新页面时会重新出现。前端组件根据 `selectorDisabled = !isLatestMsg || isLoading` 把已"过期"的表单置灰，这样不影响功能但用户能看出是"已提交"。

### 关键示例

**第一轮请求**（用户在 explorer 页面，已选 prom 数据源）：

```json
{
  "chat_id": "...",
  "query": {
    "content": "创建一条 host disk 告警规则",
    "action": {
      "key": "query_generator",
      "param": {
        "datasource_type": "prometheus",
        "datasource_id": 1
      }
    },
    "page_from": { "page": "explorer", "param": { "datasource_type": "prometheus", "datasource_id": 1 } }
  }
}
```

> 注意 `action.key="query_generator"` 是前端硬编码的入口默认值。后端的 `creation` 关键词 fast-path 会**覆盖**它，所以前端不需要为创建场景特殊处理 `actionKey`。

**第一轮响应**（halted）：

```json
{
  "dat": {
    "seq_id": 1,
    "is_finish": true,
    "response": [
      {
        "content_type": "form_select",
        "content": "{\"skill_name\":\"n9e-create-alert-rule\",\"fields\":[{\"key\":\"busi_group_id\",\"type\":\"single\",\"candidates\":[{\"id\":1,\"name\":\"Default Busi Group\",\"is_default\":true},{\"id\":2,\"name\":\"ops\"}]},{\"key\":\"datasource_id\",\"type\":\"single\",\"candidates\":[{\"id\":1,\"name\":\"prom\",\"extra\":\"prometheus\",\"is_default\":true},{\"id\":2,\"name\":\"vm-main\",\"extra\":\"prometheus\"}]}]}",
        "is_finish": true,
        "is_from_ai": true
      }
    ]
  }
}
```

**第二轮请求**（用户提交表单后前端自动重发）：

```json
{
  "chat_id": "...",
  "query": {
    "content": "创建一条 host disk 告警规则",
    "action": {
      "key": "query_generator",
      "param": {
        "datasource_type": "prometheus",
        "datasource_id": 1,
        "busi_group_id": 1
      }
    },
    "page_from": { "page": "explorer", "param": { "datasource_type": "prometheus", "datasource_id": 1 } }
  }
}
```

**第二轮响应**（创建成功）：

```json
{
  "dat": {
    "seq_id": 2,
    "is_finish": true,
    "response": [
      { "content_type": "reasoning", "content": "...", "is_finish": true, "is_from_ai": true },
      { "content_type": "markdown",  "content": "✅ 已为您创建告警规则「主机磁盘使用率过高」，详情请查看下方卡片。", "is_finish": true, "is_from_ai": true },
      {
        "content_type": "alert_rule",
        "content": "{\"id\":45,\"group_id\":1,\"group_name\":\"Default Busi Group\",\"name\":\"主机磁盘使用率过高\",\"cate\":\"prometheus\",\"datasource_id\":1,\"datasource_name\":\"prom\",\"severity\":2,\"prom_ql\":\"disk_used_percent\",\"operator\":\">\",\"threshold\":80,\"for_duration\":300,\"note\":\"主机磁盘使用率持续 5 分钟超过 80%\"}",
        "is_finish": true,
        "is_from_ai": true
      }
    ]
  }
}
```

### 行为细节

- **预选默认值**：`busi_group_id` 候选按名字启发式（"Default" / "默认"）选；`datasource_id` 候选按当前请求里已有的 id 选；`team_ids` 按已有数组里的 id 选
- **预填字段也参与渲染**：从页面上下文带过来的 `datasource_id` 不会让该字段消失，只会把对应候选标 `is_default=true`，让用户**显式确认**——避免"我在 prom 探索器打开 Copilot，但其实想给另一个 vm 数据源建告警"的隐性错误
- **关键词覆盖很保守**：必须同时满足"创建动词" + "资源名词" + "无查询反义词"，所以"查询已创建的告警规则"不会被误路由到 creation
- **多个创建场景共享 sessionParam**：用户在同一会话里先创建告警规则再创建仪表盘，不需要重新选业务组——除非新场景需要的字段还没填过

---

## 创建新会话

```
POST /api/n9e/assistant/chat/new
```

### 请求体

```json
{
  "page": "explorer",
  "param": {
    "datasource_type": "prometheus",
    "datasource_id": 1
  }
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| page | string | 否 | 页面类型，枚举值：`explorer` / `dashboards` / `alert_rule` / `alert_history` / `active_alert` / `notify_tpl` / `datasource`，详见 [AssistantPageInfo](#assistantpageinfo) |
| param | object | 否 | 页面参数，随 page 不同而不同 |

### 响应

`page` 和 `param` 会原样回写到 `page_from`，供后续消息发送和「快捷提问」前端展示使用（前端按 `page` 硬编码预置提示词，规范见 [ai-chat-recommend-action.md](./ai-chat-recommend-action.md)）。

```json
{
  "dat": {
    "chat_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "title": "New Chat",
    "last_update": 1710000000,
    "page_from": {
      "page": "explorer",
      "param": {
        "datasource_type": "prometheus",
        "datasource_id": 1
      }
    },
    "user_id": 1,
    "is_new": true
  },
  "err": ""
}
```

---

## 获取会话历史列表

```
GET /api/n9e/assistant/chat/history
```

返回当前用户的所有会话，无需参数。

### 响应

```json
{
  "dat": [
    {
      "chat_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "title": "如何查询 CPU 使用率",
      "last_update": 1710000000,
      "page_from": {
        "page": "explorer",
        "param": {}
      },
      "user_id": 1,
      "is_new": false
    }
  ],
  "err": ""
}
```

---

## 删除会话

```
DELETE /api/n9e/assistant/chat/:chatId
```

### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| chatId | string | 会话 ID |

### 响应

```json
{
  "dat": "",
  "err": ""
}
```

### 错误

- `403` 非会话所有者
- `404` 会话不存在

---

## 发送消息

```
POST /api/n9e/assistant/message/new
```

发送一条用户消息，触发 AI 异步处理。响应立即返回 `chat_id` 和 `seq_id`，前端通过轮询消息详情或订阅 SSE 流获取 AI 回复。

> **注意**：同一会话同时只能处理一条消息（Redis 锁），如果上一条消息未处理完成，会返回 `409 Conflict`。

### 请求体

```json
{
  "chat_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "query": {
    "content": "帮我写一个查询 CPU 使用率的 PromQL",
    "action": {
      "key": "query_generator",
      "param": {
        "datasource_type": "prometheus",
        "datasource_id": 1
      }
    },
    "page_from": {
      "page": "explorer",
      "param": {
        "datasource_type": "prometheus",
        "datasource_id": 1
      }
    }
  }
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| chat_id | string | 是 | 会话 ID |
| query.content | string | 是 | 用户输入内容 |
| query.action | object | 否 | 指定操作，不传时根据会话的 page_from 自动推断 |
| query.action.key | string | 否 | 操作标识，如 `query_generator` |
| query.action.param | object | 否 | 操作参数 |
| query.page_from | object | 否 | 发起消息的页面来源 |

### 响应

```json
{
  "dat": {
    "chat_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "seq_id": 1
  },
  "err": ""
}
```

### 错误

- `400` chat_id 或 query.content 为空
- `403` 非会话所有者
- `409` 会话正忙，请等待当前消息处理完成

---

## 获取消息详情

```
POST /api/n9e/assistant/message/detail
```

获取单条消息的完整状态，前端可轮询此接口获取 AI 回复进度。

### 请求体

```json
{
  "chat_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "seq_id": 1
}
```

### 响应

**处理中：**

```json
{
  "dat": {
    "chat_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "seq_id": 1,
    "query": {
      "content": "帮我写一个查询 CPU 使用率的 PromQL",
      "action": { "key": "query_generator", "param": {} },
      "page_from": { "page": "explorer", "param": {} }
    },
    "response": [
      {
        "content_type": "markdown",
        "content": "",
        "stream_id": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
        "is_finish": false,
        "is_from_ai": true
      }
    ],
    "cur_step": "Using tools...",
    "is_finish": false,
    "recommend_action": [],
    "err_code": 0,
    "err_title": "",
    "err_msg": "",
    "executed_tools": true
  },
  "err": ""
}
```

**处理完成（query_generator 操作）：**

`response` 数组最多包含三个元素：`reasoning`（思考过程，可选）、`query`（查询语句）和 `markdown`（文字说明），前端根据 `content_type` 分别渲染。

```json
{
  "dat": {
    "chat_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "seq_id": 1,
    "query": {
      "content": "帮我写一个查询 CPU 使用率的 PromQL",
      "action": { "key": "query_generator", "param": { "datasource_type": "prometheus", "datasource_id": 1 } },
      "page_from": { "page": "explorer", "param": { "datasource_type": "prometheus", "datasource_id": 1 } }
    },
    "response": [
      {
        "content_type": "reasoning",
        "content": "用户想要查询 CPU 使用率，我需要使用 cpu_usage_active 指标。这是一个 gauge 类型的指标，可以通过 cpu=\"cpu-total\" 标签聚合所有核心...",
        "is_finish": true,
        "is_from_ai": true
      },
      {
        "content_type": "query",
        "content": "cpu_usage_active{cpu=\"cpu-total\"}",
        "stream_id": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
        "is_finish": true,
        "is_from_ai": true
      },
      {
        "content_type": "markdown",
        "content": "使用 cpu-usage-active 指标查询主机 CPU 使用率。该指标是一个 gauge 类型，表示当前活跃 CPU 使用的百分比。cpu=\"cpu-total\" 标签表示该指标已聚合所有 CPU 核心的使用率。",
        "is_finish": true,
        "is_from_ai": true
      }
    ],
    "cur_step": "",
    "is_finish": true,
    "recommend_action": [],
    "err_code": 0,
    "err_title": "",
    "err_msg": "",
    "executed_tools": true
  },
  "err": ""
}
```

### 错误

- `403` 非会话所有者
- `404` 消息不存在

---

## 获取会话消息历史

```
POST /api/n9e/assistant/message/history
```

获取某个会话下的所有消息列表。

### 请求体

```json
{
  "chat_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
}
```

### 响应

```json
{
  "dat": [
    {
      "chat_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "seq_id": 1,
      "query": { "content": "帮我写一个查询 CPU 使用率的 PromQL", "action": {}, "page_from": {} },
      "response": [
        {
          "content_type": "reasoning",
          "content": "用户想要查询 CPU 使用率...",
          "is_finish": true,
          "is_from_ai": true
        },
        {
          "content_type": "query",
          "content": "cpu_usage_active{cpu=\"cpu-total\"}",
          "is_finish": true,
          "is_from_ai": true
        },
        {
          "content_type": "markdown",
          "content": "使用 cpu-usage-active 指标查询主机 CPU 使用率。",
          "is_finish": true,
          "is_from_ai": true
        }
      ],
      "cur_step": "",
      "is_finish": true,
      "recommend_action": [],
      "err_code": 0,
      "err_title": "",
      "err_msg": "",
      "executed_tools": false
    }
  ],
  "err": ""
}
```

### 错误

- `400` chat_id 为空
- `403` 非会话所有者

---

## 取消消息处理

```
POST /api/n9e/assistant/message/cancel
```

取消正在处理中的消息。

### 请求体

```json
{
  "chat_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "seq_id": 1
}
```

### 响应

```json
{
  "dat": "",
  "err": ""
}
```

取消后，该消息的 `err_code` 会被设置为 `-2`。

### 错误

- `403` 非会话所有者
- `404` 消息未在执行中或不存在

---

## 订阅流式输出（SSE）

```
POST /api/n9e/stream
```

通过 Server-Sent Events 实时接收 AI 回复的增量内容。`stream_id` 从发送消息后的消息详情中获取（`response[0].stream_id`）。

### 请求体

```json
{
  "stream_id": "b2c3d4e5-f6a7-8901-bcde-f12345678901"
}
```

### 响应格式

响应为 SSE（`Content-Type: text/event-stream`），每条数据格式如下：

**数据事件：**

```
data: {"type":"text","content":"","delta":"你可以","done":false,"timestamp":1710000000}

data: {"type":"text","content":"","delta":"使用以下","done":false,"timestamp":1710000001}
```

**结束事件：**

```
event: finish
data:
```

### StreamChunk 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| type | string | 事件类型：`thinking` / `text` / `tool_call` / `tool_result` / `done` / `error` |
| content | string | 完整内容（累积） |
| delta | string | 增量内容 |
| done | bool | 是否结束 |
| error | string | 错误信息（type 为 `error` 时） |
| timestamp | int64 | 时间戳（毫秒） |

### type 事件类型说明

| type | 说明 |
|------|------|
| `thinking` | AI 思考过程 |
| `text` | LLM 文本输出（前端应拼接 delta 实时展示） |
| `tool_call` | 正在调用工具 |
| `tool_result` | 工具返回结果 |
| `done` | 处理完成，content 包含最终完整回复 |
| `error` | 处理出错，error 字段包含错误信息 |

---

## 前端对接流程

### 1. 创建会话

```
POST /api/n9e/assistant/chat/new  →  获取 chat_id
```

### 2. 发送消息

```
POST /api/n9e/assistant/message/new  →  获取 seq_id
```

### 3. 获取 AI 回复（两种方式）

**方式 A：SSE 流式（推荐）**

1. 发送消息后，调用 `POST /api/n9e/assistant/message/detail` 获取 `stream_id`
2. 使用 `stream_id` 调用 `POST /api/n9e/stream` 建立 SSE 连接
3. 监听 `data` 事件拼接 `delta` 实时展示内容
4. 收到 `event: finish` 后关闭连接
5. 再调用一次 `message/detail` 获取最终完整消息（包含 recommend_action 等）

**方式 B：轮询**

1. 定时调用 `POST /api/n9e/assistant/message/detail`
2. 检查 `is_finish` 字段
3. 为 `true` 时停止轮询，遍历 `response` 数组按 `content_type` 分别渲染

### 4. 取消消息（可选）

```
POST /api/n9e/assistant/message/cancel
```

### 5. 查看历史

```
GET /api/n9e/assistant/chat/history    →  会话列表
POST /api/n9e/assistant/message/history →  某会话的消息列表
```

---

## v1 Service API（外部服务调用）

供外部服务（如统一 AI 网关）通过 BasicAuth 调用的接口。通过 `serviceUser` 中间件将 Header 中的用户名注入上下文，从而**直接复用前端 handler**，无需额外的 `*ByService` 函数。

区别于前端接口：

- **认证**：BasicAuth，需在配置文件中启用 `APIForService` 并配置账号密码
- **用户身份**：通过 `X-Service-Username` HTTP Header 传递最终用户名。后端中间件据此加载用户对象并注入 context，handler 通过 `c.MustGet("user")` 获取，与前端行为一致
- **基础路径**：`/v1/n9e`
- **请求体格式**：与前端接口完全一致（不需要在 body 中传 username）

### 接口列表

| 方法 | 路径 | 对应前端接口 | 说明 |
|------|------|------------|------|
| POST | `/v1/n9e/assistant/chat/new` | `POST /api/n9e/assistant/chat/new` | 创建会话 |
| GET | `/v1/n9e/assistant/chat/history` | `GET /api/n9e/assistant/chat/history` | 会话历史 |
| DELETE | `/v1/n9e/assistant/chat/:chatId` | `DELETE /api/n9e/assistant/chat/:chatId` | 删除会话 |
| POST | `/v1/n9e/assistant/message/new` | `POST /api/n9e/assistant/message/new` | 发送消息 |
| POST | `/v1/n9e/assistant/message/detail` | `POST /api/n9e/assistant/message/detail` | 消息详情 |
| POST | `/v1/n9e/assistant/message/history` | `POST /api/n9e/assistant/message/history` | 消息历史 |
| POST | `/v1/n9e/assistant/message/cancel` | `POST /api/n9e/assistant/message/cancel` | 取消消息 |
| POST | `/v1/n9e/assistant/stream` | `POST /api/n9e/stream` | SSE 流式输出 |

### 请求格式

请求体与前端接口完全一致，用户身份通过 `X-Service-Username` Header 传递（`stream` 接口除外，无需该 Header）。

**公共 Header：**

| Header | 必填 | 说明 |
|--------|------|------|
| `Authorization` | 是 | BasicAuth 凭证 |
| `X-Service-Username` | 是（stream 除外） | 最终用户的用户名，后端按该用户的权限执行操作 |
| `Content-Type` | 是 | `application/json` |

#### 创建会话

```
POST /v1/n9e/assistant/chat/new
X-Service-Username: alice
```

```json
{
  "page": "explorer",
  "param": {
    "datasource_type": "prometheus",
    "datasource_id": 1
  }
}
```

#### 获取会话历史

```
GET /v1/n9e/assistant/chat/history
X-Service-Username: alice
```

无请求体。

#### 删除会话

```
DELETE /v1/n9e/assistant/chat/:chatId
X-Service-Username: alice
```

#### 发送消息

```
POST /v1/n9e/assistant/message/new
X-Service-Username: alice
```

```json
{
  "chat_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "query": {
    "content": "当前有哪些活跃告警？",
    "action": {
      "key": "alert_query"
    },
    "page_from": {
      "page": "active_alert"
    }
  }
}
```

#### 获取消息详情

```
POST /v1/n9e/assistant/message/detail
X-Service-Username: alice
```

```json
{
  "chat_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "seq_id": 1
}
```

#### 获取消息历史

```
POST /v1/n9e/assistant/message/history
X-Service-Username: alice
```

```json
{
  "chat_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
}
```

#### 取消消息

```
POST /v1/n9e/assistant/message/cancel
X-Service-Username: alice
```

```json
{
  "chat_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "seq_id": 1
}
```

#### SSE 流式输出

```
POST /v1/n9e/assistant/stream
```

```json
{
  "stream_id": "b2c3d4e5-f6a7-8901-bcde-f12345678901"
}
```

> `stream` 接口不需要 `X-Service-Username` Header，`stream_id` 是服务端生成的临时凭证，仅通过已鉴权的 `message/detail` 接口获取。

### 调用示例（curl）

```bash
# 创建会话
curl -u service_user:password -X POST http://localhost:17000/v1/n9e/assistant/chat/new \
  -H 'Content-Type: application/json' \
  -H 'X-Service-Username: alice' \
  -d '{"page":"explorer","param":{"datasource_type":"prometheus","datasource_id":1}}'

# 发送消息
curl -u service_user:password -X POST http://localhost:17000/v1/n9e/assistant/message/new \
  -H 'Content-Type: application/json' \
  -H 'X-Service-Username: alice' \
  -d '{"chat_id":"<chat_id>","query":{"content":"当前有哪些活跃告警？"}}'

# 轮询消息状态
curl -u service_user:password -X POST http://localhost:17000/v1/n9e/assistant/message/detail \
  -H 'Content-Type: application/json' \
  -H 'X-Service-Username: alice' \
  -d '{"chat_id":"<chat_id>","seq_id":1}'

# SSE 流式获取回复
curl -u service_user:password -X POST http://localhost:17000/v1/n9e/assistant/stream \
  -H 'Content-Type: application/json' \
  -d '{"stream_id":"<stream_id>"}'
```

### 外部服务对接流程

```
服务 A                                         夜莺 AI Agent
 │                                               │
 │  1. POST /v1/n9e/assistant/chat/new           │
 │  Header: X-Service-Username: alice            │
 │  Body: {"page":"..."}                         │
 │──────────────────────────────────────────────►│
 │◄──── 返回 chat_id ───────────────────────────│
 │                                               │
 │  2. POST /v1/n9e/assistant/message/new        │
 │  Header: X-Service-Username: alice            │
 │  Body: {"chat_id":"...", "query":{...}}       │
 │──────────────────────────────────────────────►│
 │◄──── 返回 {chat_id, seq_id} ─────────────────│
 │                                               │
 │  3. POST /v1/n9e/assistant/message/detail     │
 │  Header: X-Service-Username: alice            │
 │  Body: {"chat_id":"...", "seq_id":1}          │
 │──────────────────────────────────────────────►│
 │◄──── 返回消息详情(含 stream_id) ──────────────│
 │                                               │
 │  4. POST /v1/n9e/assistant/stream             │
 │  Body: {"stream_id":"..."}                    │
 │──────────────────────────────────────────────►│
 │◄──── SSE: data {delta} ──────────────────────│
 │◄──── SSE: data {delta} ──────────────────────│
 │◄──── SSE: event: finish ─────────────────────│
 │                                               │
 │  5. POST /v1/n9e/assistant/message/detail     │
 │  (获取最终完整回复)                            │
 │──────────────────────────────────────────────►│
 │◄──── 返回完整消息 ───────────────────────────│
```
