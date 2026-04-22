# AI LLM Config API

所有接口需要管理员权限（`auth` + `admin`）。

## 数据结构

### AILLMConfig

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int64 | - | 主键，自增 |
| name | string | 是 | 配置名称 |
| description | string | 否 | 描述 |
| api_type | string | 是 | 提供商类型：`openai`、`claude`、`gemini` |
| api_url | string | 是 | API 地址 |
| api_key | string | 是 | API 密钥 |
| model | string | 是 | 模型名称 |
| extra_config | object | 否 | 高级配置，见 LLMExtraConfig |
| enabled | bool | 否 | 是否启用，请显式传入 `true` 或 `false` |
| created_at | int64 | - | 创建时间（Unix 时间戳） |
| created_by | string | - | 创建人 |
| updated_at | int64 | - | 更新时间（Unix 时间戳） |
| updated_by | string | - | 更新人 |

### LLMExtraConfig

| 字段 | 类型 | 说明 |
|------|------|------|
| timeout_seconds | int | 请求超时时间（秒），默认 30 |
| skip_tls_verify | bool | 跳过 TLS 证书校验 |
| proxy | string | HTTP 代理地址 |
| custom_headers | map[string]string | 自定义请求头 |
| custom_params | map[string]any | 自定义请求参数 |
| temperature | float64 | 生成温度（可选） |
| max_tokens | int | 最大输出 Token 数（可选） |
| context_length | int | 上下文窗口大小（可选） |

---

## 获取 LLM 配置列表

```
GET /api/n9e/ai-llm-configs
```

### 响应

```json
{
  "dat": [
    {
      "id": 1,
      "name": "gpt-4o",
      "description": "OpenAI GPT-4o",
      "api_type": "openai",
      "api_url": "https://api.openai.com",
      "api_key": "sk-xxx",
      "model": "gpt-4o",
      "extra_config": {
        "temperature": 0.7,
        "max_tokens": 4096
      },
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

## 获取 LLM 配置详情

```
GET /api/n9e/ai-llm-config/:id
```

### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | LLM 配置 ID |

### 响应

```json
{
  "dat": {
    "id": 1,
    "name": "gpt-4o",
    "description": "OpenAI GPT-4o",
    "api_type": "openai",
    "api_url": "https://api.openai.com",
    "api_key": "sk-xxx",
    "model": "gpt-4o",
    "extra_config": {
      "temperature": 0.7,
      "max_tokens": 4096
    },
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

- `404` LLM 配置不存在

---

## 创建 LLM 配置

```
POST /api/n9e/ai-llm-configs
```

### 请求体

```json
{
  "name": "gpt-4o",
  "description": "OpenAI GPT-4o",
  "api_type": "openai",
  "api_url": "https://api.openai.com",
  "api_key": "sk-xxx",
  "model": "gpt-4o",
  "extra_config": {
    "timeout_seconds": 60,
    "temperature": 0.7,
    "max_tokens": 4096,
    "custom_headers": {
      "X-Custom": "value"
    }
  },
  "enabled": true
}
```

### 校验规则

- `name`、`api_type`、`api_url`、`api_key`、`model` 均为必填

### 响应

```json
{
  "dat": 1,
  "err": ""
}
```

返回新创建的配置 ID。

---

## 更新 LLM 配置

```
PUT /api/n9e/ai-llm-config/:id
```

### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | LLM 配置 ID |

### 请求体

同创建接口。**注意：如果 `api_key` 为空，则保留原值不更新。**

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

- `404` LLM 配置不存在

---

## 删除 LLM 配置

```
DELETE /api/n9e/ai-llm-config/:id
```

### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | LLM 配置 ID |

### 响应

```json
{
  "dat": "",
  "err": ""
}
```

### 错误

- `404` LLM 配置不存在

---

## 测试 LLM 连接

无需先创建配置，直接传入连接参数进行连通性测试。

```
POST /api/n9e/ai-llm-config/test
```

### 请求体

```json
{
  "api_type": "openai",
  "api_url": "https://api.openai.com",
  "api_key": "sk-xxx",
  "model": "gpt-4o",
  "extra_config": {
    "timeout_seconds": 30,
    "skip_tls_verify": false,
    "proxy": "",
    "custom_headers": {}
  }
}
```

### 校验规则

- `api_type`、`api_url`、`api_key`、`model` 均为必填

### 测试行为

根据 `api_type` 向对应的 API 发送一个最小请求（"Hi"，max_tokens=5）：

| api_type | 请求地址 | 认证方式 |
|----------|---------|---------|
| openai | `{api_url}/chat/completions` | `Authorization: Bearer {api_key}` |
| claude | `{api_url}/v1/messages` | `x-api-key: {api_key}` |
| gemini | `{api_url}/v1beta/models/{model}:generateContent?key={api_key}` | URL 参数 |

### 响应

成功：

```json
{
  "dat": {
    "success": true,
    "duration_ms": 856
  },
  "err": ""
}
```

失败：

```json
{
  "dat": {
    "success": false,
    "duration_ms": 5000
  },
  "err": "HTTP 401: {\"error\": \"invalid api key\"}"
}
```
