# AI Agent API

所有接口需要管理员权限（`auth` + `admin`）。

## 数据结构

### AIAgent

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int64 | - | 主键，自增 |
| name | string | 是 | Agent 名称 |
| description | string | 否 | 描述 |
| use_case | string | 否 | 用途场景，如 `chat` |
| llm_config_id | int64 | 是 | 关联的 LLM 配置 ID |
| skill_ids | int64[] | 否 | 关联的 Skill ID 列表 |
| mcp_server_ids | int64[] | 否 | 关联的 MCP Server ID 列表 |
| enabled | bool | 否 | 是否启用，请显式传入 `true` 或 `false` |
| created_at | int64 | - | 创建时间（Unix 时间戳） |
| created_by | string | - | 创建人 |
| updated_at | int64 | - | 更新时间（Unix 时间戳） |
| updated_by | string | - | 更新人 |
| llm_config_name | string | - | 运行时字段，关联的 LLM 配置名称（不存储） |

---

## 获取 Agent 列表

```
GET /api/n9e/ai-agents
```

### 响应

```json
{
  "dat": [
    {
      "id": 1,
      "name": "chat-agent",
      "description": "AI 对话 Agent",
      "use_case": "chat",
      "llm_config_id": 1,
      "skill_ids": [1, 2],
      "mcp_server_ids": [1],
      "enabled": true,
      "created_at": 1710000000,
      "created_by": "admin",
      "updated_at": 1710000000,
      "updated_by": "admin"
    }
  ],
  "err": ""
}
```

---

## 获取 Agent 详情

```
GET /api/n9e/ai-agent/:id
```

### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | Agent ID |

### 响应

```json
{
  "dat": {
    "id": 1,
    "name": "chat-agent",
    "description": "AI 对话 Agent",
    "use_case": "chat",
    "llm_config_id": 1,
    "skill_ids": [1, 2],
    "mcp_server_ids": [1],
    "enabled": true,
    "created_at": 1710000000,
    "created_by": "admin",
    "updated_at": 1710000000,
    "updated_by": "admin"
  },
  "err": ""
}
```

### 错误

- `404` Agent 不存在

---

## 创建 Agent

```
POST /api/n9e/ai-agents
```

### 请求体

```json
{
  "name": "chat-agent",
  "description": "AI 对话 Agent",
  "use_case": "chat",
  "llm_config_id": 1,
  "skill_ids": [1, 2],
  "mcp_server_ids": [1],
  "enabled": true
}
```

### 校验规则

- `name` 必填
- `llm_config_id` 必填，且大于 0

### 响应

```json
{
  "dat": 1,
  "err": ""
}
```

返回新创建的 Agent ID。

---

## 更新 Agent

```
PUT /api/n9e/ai-agent/:id
```

### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | Agent ID |

### 请求体

```json
{
  "name": "chat-agent-v2",
  "description": "更新后的描述",
  "use_case": "chat",
  "llm_config_id": 2,
  "skill_ids": [1, 3],
  "mcp_server_ids": [],
  "enabled": true
}
```

### 校验规则

同创建接口。

### 响应

```json
{
  "dat": "",
  "err": ""
}
```

### 错误

- `404` Agent 不存在

---

## 删除 Agent

```
DELETE /api/n9e/ai-agent/:id
```

### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | Agent ID |

### 响应

```json
{
  "dat": "",
  "err": ""
}
```

### 错误

- `404` Agent 不存在

