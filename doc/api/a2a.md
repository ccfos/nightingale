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

依赖前置：A2A / MCP 鉴权复用 `[HTTP.TokenAuth]`，需保证后者已启用。

## 二、对外端点

| 端点 | 鉴权 | 说明 |
|---|---|---|
| `GET /.well-known/agent.json` | 无 | A2A AgentCard，公开发现 |
| `ANY /a2a/*` | `X-User-Token` | A2A REST + JSON-RPC + Streaming |
| `ANY /mcp/*` | `X-User-Token` | MCP Streamable HTTP |

鉴权完全复用 n9e 现有 `tokenAuth()` 中间件，与 `/api/n9e/*` 行为一致。AgentCard 中通过 `securitySchemes` 声明：

```json
{
  "securitySchemes": {
    "x-user-token": { "type": "apiKey", "in": "header", "name": "X-User-Token" }
  },
  "security": [{ "x-user-token": [] }]
}
```

## 三、概念映射

| A2A | n9e |
|---|---|
| `ContextID` | `assistant_chat.chat_id`（不存在则建） |
| `TaskID` | 单次 message 执行（`AssistantMessage.seq_id`） |
| `Message.Parts[*].Text` | `MessageQuery.Content` |
| `Metadata.model_id` | 覆盖默认 LLM Config |
| `TaskStatusUpdateEvent` | working / failed / completed / canceled |
| `TaskArtifactUpdateEvent` | 文本流（思考 + 答案），由 `aiagent.StreamChunk` 桥接 |
| `Cancel` | `assistantMessageCancel`（释放 ChatLock + cancel ctx） |

## 四、模块布局

```
aiagent/a2a/
  agent_card.go     # AgentCard 构造 + securitySchemes 声明
  executor.go       # a2asrv.AgentExecutor 实现（Execute / Cancel）
  bridge.go         # StreamChunk → a2a.Event 转换
  user_ctx.go       # userCtxKey + helper
  mcp.go            # MCP Server 入口（单工具 n9e_assistant）

center/router/
  router_a2a.go     # 路由注册 + injectUserToCtx 中间件
  router.go         # 调用 registerA2A
  router_ai_assistant.go  # 抽出 createAssistantMessage / runAssistantMessage / cancelAssistantMessage
```

## 五、Executor 流程

1. 中间件 `tokenAuth` → `user` → `injectUserToCtx` 把 `*models.User` 注入 `request.Context`
2. `Execute()` 入口：
   - 解析 `Message.Parts` 拼接成文本
   - 用 `ContextID` 获取或创建 `assistant_chat`
   - 调用现有 `createAssistantMessage` 落库 + 占座 streamID + 写 streamBus init
   - 启动现有 `processAssistantMessage` 跑 ReAct
   - 主协程订阅 streamBus，把 `StreamChunk` 桥接成 A2A 事件
3. `Cancel()`：调用现有 `cancelAssistantMessage`，释放 ChatLock + cancel parent ctx，然后 yield `TaskStateCanceled`

### Bridge 状态机

| StreamChunk.Type | A2A Event |
|---|---|
| `reasoning` | `NewArtifactEvent(textPartThought(...))`（携带 `adk_thought` metadata，兼容 ADK） |
| 工具调用开始 | `NewStatusUpdateEvent(working, "calling tool xxx")` |
| `message` 增量 | `NewArtifactUpdateEvent(artifactID, NewTextPart(delta))` |
| `done` | `NewArtifactUpdateEvent(LastChunk=true)` + `NewStatusUpdateEvent(completed)` |
| 错误 | `NewStatusUpdateEvent(failed, errorText)` |

## 六、MCP

P1 仅注册一个 tool，包装内置助手：

```jsonc
{
  "name": "n9e_assistant",
  "description": "Operate the Nightingale platform via natural language.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "message":  { "type": "string" },
      "model_id": { "type": "integer" }
    },
    "required": ["message"]
  }
}
```

P2 再考虑将 `aiagent/builtin_tools.go` 中的查询类工具逐个独立暴露。

## 七、A2A 规范覆盖度

| 项 | 状态 |
|---|---|
| Executor 接口 (`Execute`/`Cancel`) | P1 |
| 流式事件（artifact/status update） | P2 |
| Cancel 真正打断（释放 ChatLock + cancel ctx） | P2 |
| AgentCard SecuritySchemes（X-User-Token） | P0 |
| TaskStore 持久化（`tasks/get` / `resubscribe`） | P3 |
| Push Notifications | P4（按需） |

## 八、分阶段交付

| 阶段 | 范围 | 验收 |
|---|---|---|
| P0 骨架 | 配置 + 路由 + AgentCard + 鉴权 + 占位 executor | `curl /.well-known/agent.json` 返回正确 card |
| P1 非流式 | Execute 同步问答 | `message/send` 跑通 |
| P2 流式 + Cancel | streamBus → artifact 事件；真 cancel | `message/stream` SSE；`tasks/cancel` 打断 |
| P3 ContextID 续接 + TaskStore | 多轮 + `tasks/get` / `resubscribe` | 断线重连恢复 task |
| P4 MCP | MCP Streamable HTTP 单工具 | MCP Inspector / Claude Desktop 可调通 |
| P5 观测 + 限流 | metrics + per-user 限速 | dashboard + 压测 |

## 九、风险点

1. **长 idle 心跳**：n9e ReAct 单轮可能跑十几分钟，bridge 周期性发 `working` status update（30s）避免 LB / 客户端断连。
2. **ChatLock 冲突**：同 `ContextID` 并发时返回 `TaskState=rejected`。
3. **a2a-go 版本对齐**：锁定与 fc-model-server 同步的 commit 防 API 漂移。
4. **首次发布**：release notes 显式写明默认开启 `/a2a/*` `/mcp/*` 端点（虽鉴权），让企业用户按需 `Disable = true`。

## 十、客户端调用示例

```bash
# 1) discover
curl https://n9e.example.com/.well-known/agent.json

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
