# AI Chat API

AI 对话助手接口，支持多轮对话、流式输出、消息取消等功能。所有接口需要登录（`auth` + `user`）。

基础路径：`/api/n9e`

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
- `query`：渲染为带语法高亮的 QueryResultBlock，附带「复制」和「执行查询」按钮
- `markdown`：直接按 Markdown 渲染文字说明
- `hint`：渲染为提示样式

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
| recommend_action | AssistantAction[] | 推荐操作列表 |
| user_id | int64 | 所属用户 ID |
| is_new | bool | 是否为新会话（尚未发送消息） |

### AssistantPageInfo

记录用户从哪个页面打开 AI 助手。`param` 为页面级别的上下文信息，不同页面携带的字段不同，部分页面可能不需要传。

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| page | string | 否 | 页面类型，可选值：`dashboards` / `alert_history` / `active_alert` / `explorer` |
| param | object | 否 | 页面级上下文，结构因页面而异，不需要时可省略。示例见下方 |

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
| content_type | string | 内容类型：`markdown` / `query` / `hint`（见下方说明） |
| content | string | 回复内容（处理完成后填充） |
| stream_id | string | 流式 ID，用于订阅 SSE 流（仅第一个元素携带） |
| is_finish | bool | 该条回复是否已完成 |
| is_from_ai | bool | 是否来自 AI |

**content_type 说明：**

| 值 | 说明 | content 格式 |
|------|------|------|
| `markdown` | 文字说明 | Markdown 文本，直接渲染 |
| `query` | 查询语句（PromQL / SQL） | 纯查询语句字符串，前端渲染为代码块 + 复制/执行按钮 |
| `hint` | 提示信息 | 提示文本 |

对于 `query_generator` 类型的操作，AI 处理完成后 `response` 数组会包含两个元素：
1. **`query` 元素**：纯查询语句（如 `cpu_usage_active{cpu="cpu-total"}`），前端可直接用于执行查询
2. **`markdown` 元素**：查询的文字说明（如"使用 cpu-usage-active 指标查询主机 CPU 使用率..."）

如果 AI 返回的内容无法解析为结构化的 query+explanation JSON，则回退为单个 `markdown` 元素。

### AssistantAction

| 字段 | 类型 | 说明 |
|------|------|------|
| content | string | 操作展示文案 |
| key | string | 操作标识，如 `query_generator` |
| param | AssistantActionParam | 操作参数 |

### AssistantActionParam

| 字段 | 类型 | 说明 |
|------|------|------|
| datasource_type | string | 数据源类型（可选） |
| datasource_id | int64 | 数据源 ID（可选） |
| database_name | string | 数据库名（可选） |
| table_name | string | 表名（可选） |

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
| page | string | 否 | 页面类型：`dashboards` / `alert_history` / `active_alert` / `explorer` |
| param | object | 否 | 页面参数 |

### 响应

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
    "recommend_action": [],
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
      "recommend_action": [],
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

`response` 数组包含两个元素：`query`（查询语句）和 `markdown`（文字说明），前端根据 `content_type` 分别渲染。

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
