# MCP Server API

所有接口需要管理员权限（`auth` + `admin`）。

## 数据结构

### MCPServer

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int64 | - | 主键，自增 |
| name | string | 是 | 名称 |
| url | string | 是 | MCP Server 地址 |
| headers | map[string]string | 否 | 自定义 HTTP 请求头，用于认证等 |
| description | string | 否 | 描述 |
| enabled | bool | 否 | 是否启用，请显式传入 `true` 或 `false` |
| created_at | int64 | - | 创建时间（Unix 时间戳） |
| created_by | string | - | 创建人 |
| updated_at | int64 | - | 更新时间（Unix 时间戳） |
| updated_by | string | - | 更新人 |

---

## 获取 MCP Server 列表

```
GET /api/n9e/mcp-servers
```

### 响应

```json
{
  "dat": [
    {
      "id": 1,
      "name": "my-mcp-server",
      "url": "https://mcp.example.com/sse",
      "headers": {
        "Authorization": "Bearer xxx"
      },
      "description": "示例 MCP Server",
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

## 获取 MCP Server 详情

```
GET /api/n9e/mcp-server/:id
```

### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | MCP Server ID |

### 响应

```json
{
  "dat": {
    "id": 1,
    "name": "my-mcp-server",
    "url": "https://mcp.example.com/sse",
    "headers": {
      "Authorization": "Bearer xxx"
    },
    "description": "示例 MCP Server",
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

- `404` MCP Server 不存在

---

## 创建 MCP Server

```
POST /api/n9e/mcp-servers
```

### 请求体

```json
{
  "name": "my-mcp-server",
  "url": "https://mcp.example.com/sse",
  "headers": {
    "Authorization": "Bearer xxx"
  },
  "description": "示例 MCP Server",
  "enabled": true
}
```

### 校验规则

- `name` 必填（自动 trim）
- `url` 必填（自动 trim）

### 响应

```json
{
  "dat": 1,
  "err": ""
}
```

返回新创建的 MCP Server ID。

---

## 更新 MCP Server

```
PUT /api/n9e/mcp-server/:id
```

### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | MCP Server ID |

### 请求体

同创建接口。

### 可更新字段

`name`、`url`、`headers`、`description`、`enabled`。

### 响应

```json
{
  "dat": "",
  "err": ""
}
```

### 错误

- `404` MCP Server 不存在

---

## 删除 MCP Server

```
DELETE /api/n9e/mcp-server/:id
```

### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | MCP Server ID |

### 响应

```json
{
  "dat": "",
  "err": ""
}
```

### 错误

- `404` MCP Server 不存在

---

## 测试 MCP Server 连接

无需先创建，直接传入连接参数进行连通性测试。通过 MCP 协议初始化握手并获取工具列表来验证连通性。

```
POST /api/n9e/mcp-server/test
```

### 请求体

```json
{
  "url": "https://mcp.example.com/sse",
  "headers": {
    "Authorization": "Bearer xxx"
  }
}
```

### 校验规则

- `url` 必填

### 测试行为

1. 发送 `initialize` 请求（协议版本 `2024-11-05`）
2. 发送 `notifications/initialized` 通知
3. 发送 `tools/list` 请求获取工具列表

支持 JSON 和 SSE（`text/event-stream`）两种响应格式。

### 响应

成功：

```json
{
  "dat": {
    "success": true,
    "duration_ms": 320,
    "tool_count": 5
  },
  "err": ""
}
```

失败：

```json
{
  "dat": {
    "success": false,
    "duration_ms": 5000,
    "tool_count": 0,
    "error": "initialize: HTTP 403: Forbidden"
  },
  "err": ""
}
```

---

## 获取 MCP Server 工具列表

获取已创建的 MCP Server 提供的工具列表。

```
GET /api/n9e/mcp-server/:id/tools
```

### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | MCP Server ID |

### 响应

```json
{
  "dat": [
    {
      "name": "query_database",
      "description": "Execute a SQL query against the database"
    },
    {
      "name": "search_logs",
      "description": "Search through application logs"
    }
  ],
  "err": ""
}
```

### 错误

- `404` MCP Server 不存在
