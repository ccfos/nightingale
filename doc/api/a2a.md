# A2A / MCP 协议支持

n9e 内置 AI 助手对外以 [A2A（Agent-to-Agent Protocol）](https://a2a-protocol.org/) 标准暴露，
使任何遵循 A2A 的客户端（Claude、ADK、其他 Agent 编排平台）都能调用 n9e 的智能体能力
（查告警/事件/指标/资源、跑诊断、巡检大盘等）。同时附带 MCP（Model Context Protocol）
Streamable HTTP 入口，供 MCP 生态使用。

实现复用现有 `aiagent.Agent` 内核，不做并行实现。

## 一、配置

```toml
[HTTP.A2A]
# Disable = false       # 默认 false = 开启
# DisableMCP = false    # 默认 false = 开启
# BaseURL = ""          # 可选，留空自动从请求 Host + X-Forwarded-Proto 推断
```

| 字段 | 类型 | 默认 | 说明 |
|---|---|---|---|
| Disable | bool | false | 关掉 A2A + MCP 全部对外端点 |
| DisableMCP | bool | false | 仅关掉 MCP 入口，A2A 不受影响 |
| BaseURL | string | "" | AgentCard 中暴露的绝对 URL；留空时按请求自动推断 |

配置结构定义见 `pkg/httpx/httpx.go` 的 `A2AConfig`。

依赖前置：A2A / MCP 鉴权复用 `[HTTP.TokenAuth]`，需保证后者已启用。
若 `HTTP.A2A` 未关闭但 `HTTP.TokenAuth` 未启用，启动时会打印 warning 提示
所有 `/a2a/*` `/mcp/*` 请求都会被拒绝。

## 二、对外端点

| 端点 | 鉴权 | 说明 |
|---|---|---|
| `GET /.well-known/agent-card.json` | 无 | A2A AgentCard 公开发现（v0.3+ 规范路径） |
| `GET /.well-known/agent.json` | 无 | 同上，旧客户端兼容别名 |
| `ANY /a2a` `/a2a/*` | `X-User-Token` | A2A REST + JSON 接口（含 SSE 流） |
| `ANY /mcp` `/mcp/*` | `X-User-Token` | MCP Streamable HTTP，stateless 模式 |

鉴权完全复用 n9e 现有 `tokenAuth()` 中间件，与 `/api/n9e/*` 行为一致。AgentCard 中通过
`securitySchemes` 声明（`securitySchemeName = "x-user-token"`）：

```json
{
  "securitySchemes": {
    "x-user-token": { "type": "apiKey", "in": "header", "name": "X-User-Token" }
  },
  "security": [{ "x-user-token": [] }]
}
```

`name` 字段值取自 `HTTP.TokenAuth.HeaderUserTokenKey`（默认 `X-User-Token`）。

AgentCard 同时声明：
- `Capabilities.Streaming = true`
- 一个 `n9e-assistant` Skill，附带若干中文示例 prompt
- `DefaultInputModes` / `DefaultOutputModes` 都是 `text`

## 三、概念映射

| A2A | n9e |
|---|---|
| `ContextID` | `assistant_chat.chat_id`（不存在或非本人则忽略，分配新 chat） |
| `TaskID` | 由 SDK 生成，落库时通过 `Task.Metadata` 绑定到 `(chat_id, seq_id)` |
| `Message.Parts[*].Text` | 拼接为 `MessageQuery.Content` |
| `Metadata.lang` | 强制 LLM 输出语言 |
| `Metadata.page` | `AssistantPageInfo.Page`（仅在新建 chat 时使用） |
| `TaskStatusUpdateEvent` | submitted / working / completed / failed / canceled |
| `TaskArtifactUpdateEvent` | 文本流（思考 + 答案），由 `aiagent.StreamMessage` 桥接 |
| `Cancel` | `CancelAssistantMessageInternal`（cancel ctx + 标记 + Finish stream） |

`Task.Metadata` 用两个保留键把 SDK 生成的 TaskID 绑定回 n9e 实体，供 Cancel 精准定位：

| Key | 类型 | 说明 |
|---|---|---|
| `n9e.chat_id` | string | 对应 `assistant_chat.chat_id` |
| `n9e.seq_id`  | int64  | 对应 `assistant_message.seq_id` |

## 四、模块布局

```
aiagent/a2a/
  agent_card.go          # AgentCard 构造 + securitySchemes 声明
  handler.go             # NewHTTPHandler：装配 a2asrv RequestHandler / RESTHandler
  executor.go            # a2asrv.AgentExecutor 实现（Execute / Cancel / heartbeat / terminalState）
  bridge.go              # StreamMessage → a2a.Event 转换（content/reason/step）
  user_ctx.go            # WithUser / UserFromContext
  mcp.go                 # MCP Streamable HTTP server（单工具 n9e_assistant）
  executor_test.go       # 端到端 fake-backend 单测
  taskstore/
    redis_store.go       # a2asrv/taskstore.Store 的 Redis 实现（Lua-CAS）
    redis_store_test.go

center/router/
  router_a2a.go                  # configRegisterA2A：路由挂载 + a2aBackend 适配 + 中间件
  router_ai_assistant.go         # 现有 HTTP 入口，复用 internal 共享逻辑
  router_ai_assistant_internal.go # EnsureAssistantChat / StartAssistantMessage /
                                  # CancelAssistantMessageInternal / clearWriteDeadline /
                                  # ErrMessageNotInflight 等共享导出符号

cmd/a2a-cli/
  main.go                # 终端测试客户端，使用官方 a2a-go SDK
```

`aiagent/a2a` 通过 `AssistantBackend` 接口与 `center/router` 解耦——前者绝不直接 import gin
或 router 包，后者通过 `a2aBackend` 适配器把 `*Router` 转成接口。

```go
type AssistantBackend interface {
    EnsureAssistantChat(userID int64, chatID string, page AssistantPageInfo) (*AssistantChat, error)
    StartAssistantMessage(userID int64, chat *AssistantChat, query AssistantMessageQuery, lang string) (*MessageStartResult, int, error)
    CancelAssistantMessage(ctx context.Context, chatID string, seqID int64) error
    CheckChatOwner(chatID string, userID int64) error
    StreamBus() aiagent.StreamBus
    MessageSnapshot(ctx context.Context, chatID string, seqID int64) (*AssistantMessage, error)
}
```

## 五、Executor 流程

中间件链：`tokenAuth` → `user` → `injectA2AUser` → `streamingDeadline`。
`injectA2AUser` 把 `*models.User` 注入 `request.Context`；`streamingDeadline`
通过 `http.NewResponseController.SetWriteDeadline(zero)` 清掉默认 40s
write timeout，否则长 ReAct 流会中途 `i/o timeout`。

`Execute()`：

1. 从 ctx 取 user，校验 `Message` 非空、parts 文本拼接非空
2. 解析 `Metadata.page` / `Metadata.lang`
3. `EnsureAssistantChat`：传入的 `ContextID` 仅在归属当前用户时复用，否则一律分配新 UUID
   并新建 chat（防 ID 抢占 + 跨租户存在性探测）
4. `StartAssistantMessage`：占 ChatLock、分配 seqID、写 `assistant_message`、初始化 streamBus、
   起 goroutine 跑 `processAssistantMessage`
5. `yield`：
   - `NewSubmittedTask`，`Metadata` 写入 `n9e.chat_id` / `n9e.seq_id`
   - `NewStatusUpdateEvent(working, nil)`
   - 进入 streamBus 转发循环
6. 终止时调用 `terminalState()`：读 `MsgStateGet` 快照
   - `ErrCode == MessageStatusCancel` → `Canceled`
   - `ErrCode != 0` → `Failed`
   - 其他（含 snapshot 已 TTL 失效） → `Completed`
7. `bridge.Finalize(state, errMsg)` 发终态 status update

`Cancel()`：

1. 从 `ec.StoredTask.Metadata` 取出 `(chat_id, seq_id)`，缺失或畸形 → `ErrTaskNotFound`
2. 若 `ec.ContextID` 非空且与 `chat_id` 不一致 → `ErrTaskNotFound`（防探测）
3. `CheckChatOwner` 失败 → `ErrTaskNotFound`（不区分"不存在"和"非本人"）
4. `CancelAssistantMessage`：底层若返回 `ErrMessageNotInflight` 也映射为 `ErrTaskNotFound`，
   其他错误以 `Failed` status update 回报
5. 成功后 `yield` `NewStatusUpdateEvent(canceled, nil)`

### Bridge 状态机

`aiagent.StreamMessage` 三种 phase（`StreamMessage.P`）：

| StreamMessage.P | A2A Event |
|---|---|
| `content` | 首条 `NewArtifactEvent(NewTextPart(delta))`，记 artifactID；后续同 ID 的 `NewArtifactUpdateEvent` |
| `reason`  | 同上，但每个 `TextPart` 通过 `SetMeta("adk_thought", true)` 标注（ADK 客户端识别为"思考链"） |
| `step`    | `NewStatusUpdateEvent(working, NewMessage(role=agent, NewTextPart(stepText)))`；同时清掉 reasoningArtifactID 让下一段思考开新 artifact |

终态显式 **不发** 单独的 `LastChunk` artifact-update：a2a-go SDK 的 `taskupdate.Manager`
会拒绝空 Parts 的 ArtifactUpdate（`ErrInvalidAgentResponse "artifact cannot be empty"`），
那会让 task 错误地翻成 `failed`。SDK 中 terminal status update 本身就是流结束信号，
客户端见到终态后再无 delta 即视为该 artifact 完成。

### 心跳

`heartbeatInterval = 30s`（见 `executor.go`）。和 stream 在同一 goroutine 里 `select`：
没有真实事件持续 30s 时发一次空 `NewStatusUpdateEvent(working, nil)`，规避 nginx / ALB
默认 60s idle timeout。每次真实事件都 `ticker.Reset` 一次，让心跳只在真正空闲时触发。

不开第二个 goroutine 是因为 `iter.Seq2` 的 `yield` 不允许并发调用。

## 六、TaskStore

实现位于 `aiagent/a2a/taskstore/redis_store.go`，挂在 `a2asrv.NewHandler(WithTaskStore(...))`，
保证 `tasks/get` / `tasks/resubscribe` 跨进程重启 + 跨实例 LB 路由都能拿到任务状态。

- Redis Key（24h TTL，与 streamBus 对齐）：
  - `a2a:task:<taskID>` (Hash) —— task JSON + version + user + context_id + updated
  - 未实现 `tasks/list`，因此不维护 per-user / per-context 二级索引
- Update 走 Lua CAS：检查 `prev_version` → 匹配则原子地 HSET + 版本 +1 + 刷新 TTL
- 多实例：所有 center 实例共享 Redis，CAS 保证只有一个实例能成功提交并发更新
- `UserResolver` 由 `router_a2a.go` 注入，从 ctx 取 `*models.User` 返回 `Username`，
  Get 时用于校验 task 归属（防跨租户读其他人 task）
- 不持久化到 MySQL：超过 TTL 后任务从 Redis 消失；UI 路径仍可通过 `assistant_message` 表
  查到对应消息（A2A 客户端则得到 `task_not_found`）
- **不实现 `tasks/list`**：A2A 规范允许返回 `UnsupportedOperation`，主动列举跨用户任务对
  内置助手场景没有意义，避免泄露其他用户活动

## 七、MCP

P1 仅注册一个 tool，包装内置助手：

```jsonc
// mcp.Tool
{
  "name": "n9e_assistant",
  "description": "Operate the Nightingale platform via natural language. Returns the assistant's final response as text.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "message":  { "type": "string", "description": "User request or question for the n9e assistant." },
      "chat_id":  { "type": "string", "description": "Optional existing chat ID; when omitted a new chat is created." }
    },
    "required": ["message"]
  }
}
```

输出结构：

```jsonc
// mcpOutput
{
  "content": "...final assistant text...",
  "chat_id": "uuid",
  "seq_id":  3
}
```

实现要点：
- `mcp.NewStreamableHTTPHandler` 以 `Stateless: true` 启用——每次请求新建 server 实例，
  不依赖 MCP session 状态，与 n9e 多实例部署天然兼容
- 工具内部走 `EnsureAssistantChat` + `StartAssistantMessage`，然后只 drain `P=="content"`
  的 stream message，把最终文本返回；reason / step 帧不返回给 MCP 调用者（自然语言进 / 自然语言出）

P2 再考虑将 `aiagent/builtin_tools.go` 中的查询类工具逐个独立暴露。

## 八、A2A 规范覆盖度

| 项 | 状态 |
|---|---|
| Executor 接口 (`Execute`/`Cancel`) | ✅ |
| 流式事件（artifact / status update） | ✅ |
| Heartbeat（30s 空闲发 working） | ✅ |
| Cancel 真正打断（cancel ctx + 标记 + Finish stream） | ✅ |
| AgentCard SecuritySchemes（X-User-Token） | ✅ |
| TaskStore 持久化（`tasks/get` / `tasks/resubscribe`） | ✅ — Redis 实现，Lua-CAS 多实例安全，TTL 24h |
| `tasks/list` | ❌ 故意不实现，按规范返回 `UnsupportedOperation` |
| Push Notifications | ⏳（按需） |

## 九、风险点

1. **长 idle 心跳**：n9e ReAct 单轮可能跑十几分钟，executor 在流空闲 30s 时发一次
   `TaskStateWorking` 空 status update，规避 nginx / ALB 默认 60s idle timeout。
2. **Write deadline**：`http.Server.WriteTimeout`（默认 40s）也会切流。`streamingDeadline`
   中间件统一清掉，A2A SSE 与 MCP 长响应都受益。
3. **ChatLock 冲突**：同 `ContextID` 并发时 `StartAssistantMessage` 返回 409，executor 把它
   作为普通错误回流。
4. **TaskID 与 chat 解绑风险**：Cancel 走 `Task.Metadata` 而不是"chat 内最新 seq"，避免
   `cancel + 紧跟 message:send` 之间的 race（旧策略可能误杀刚启动的新 task）。
6. **首次发布**：release notes 显式写明默认开启 `/a2a/*` `/mcp/*` 端点（虽鉴权），
   让企业用户按需 `Disable = true`。

## 十、客户端调用示例

仓库自带 `cmd/a2a-cli`，是基于官方 a2a-go SDK 的命令行测试工具：

```bash
# 流式聊天
go run ./cmd/a2a-cli \
    --server http://127.0.0.1:17000 \
    --token  <X-User-Token> \
    --message "查看 prod 业务组当前正在告警的事件"

# 续接同一个 chat
go run ./cmd/a2a-cli \
    --server http://127.0.0.1:17000 \
    --token <token> \
    --context-id <chat-id> \
    --message "进一步分析其中第一条"

# 跑完后再 tasks/get 验证 TaskStore 跨连接可恢复
go run ./cmd/a2a-cli --server http://127.0.0.1:17000 --token <t> --message hi --get
```

直接 curl 也可以：

```bash
# 1) discover
curl https://n9e.example.com/.well-known/agent-card.json

# 2) 发消息（非流式）
curl -X POST https://n9e.example.com/a2a/v1/messages:send \
  -H "Content-Type: application/json" \
  -H "X-User-Token: <user_token>" \
  -d '{
    "message": {
      "role": "user",
      "parts": [{ "text": "查看 prod 业务组当前正在告警的事件" }]
    }
  }'

# 3) 续接同一 chat（多轮）
curl -X POST https://n9e.example.com/a2a/v1/messages:send \
  -H "Content-Type: application/json" \
  -H "X-User-Token: <user_token>" \
  -d '{
    "context_id": "chat-xxx",
    "message": {
      "role": "user",
      "parts": [{ "text": "进一步分析其中第一条" }]
    }
  }'
```
