# AI LLM Config API

All endpoints require administrator privileges (`auth` + `admin`).

## Data structures

### AILLMConfig

| Field | Type | Required | Description |
|------|------|------|------|
| id | int64 | - | Primary key, auto-increment |
| name | string | Yes | Configuration name |
| description | string | No | Description |
| api_type | string | Yes | Provider type: `openai`, `claude`, `gemini` |
| api_url | string | Yes | API address |
| api_key | string | Yes | API key |
| model | string | Yes | Model name |
| extra_config | object | No | Advanced settings, see LLMExtraConfig |
| enabled | bool | No | Whether the configuration is enabled; pass `true` or `false` explicitly |
| created_at | int64 | - | Creation time (Unix timestamp) |
| created_by | string | - | Creator |
| updated_at | int64 | - | Last update time (Unix timestamp) |
| updated_by | string | - | Last updated by |

### LLMExtraConfig

| Field | Type | Description |
|------|------|------|
| timeout_seconds | int | Request timeout in seconds, default 30 |
| skip_tls_verify | bool | Skip TLS certificate verification |
| proxy | string | HTTP proxy address |
| custom_headers | map[string]string | Custom request headers |
| custom_params | map[string]any | Custom request parameters |
| temperature | float64 | Sampling temperature (optional) |
| max_tokens | int | Maximum output tokens (optional) |
| context_length | int | Context window size (optional) |

---

## List LLM configurations

```
GET /api/n9e/ai-llm-configs
```

### Response

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

## Get LLM configuration details

```
GET /api/n9e/ai-llm-config/:id
```

### Path parameters

| Parameter | Type | Description |
|------|------|------|
| id | int64 | LLM configuration ID |

### Response

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

### Errors

- `404` the LLM configuration does not exist

---

## Create an LLM configuration

```
POST /api/n9e/ai-llm-configs
```

### Request body

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

### Validation rules

- `name`, `api_type`, `api_url`, `api_key`, and `model` are all required

### Response

```json
{
  "dat": 1,
  "err": ""
}
```

Returns the ID of the newly created configuration.

---

## Update an LLM configuration

```
PUT /api/n9e/ai-llm-config/:id
```

### Path parameters

| Parameter | Type | Description |
|------|------|------|
| id | int64 | LLM configuration ID |

### Request body

Same as the create endpoint. **Note: if `api_key` is empty, the existing value is kept instead of being overwritten.**

### Validation rules

Same as the create endpoint.

### Response

```json
{
  "dat": "",
  "err": ""
}
```

### Errors

- `404` the LLM configuration does not exist

---

## Delete an LLM configuration

```
DELETE /api/n9e/ai-llm-config/:id
```

### Path parameters

| Parameter | Type | Description |
|------|------|------|
| id | int64 | LLM configuration ID |

### Response

```json
{
  "dat": "",
  "err": ""
}
```

### Errors

- `404` the LLM configuration does not exist

---

## Test an LLM connection

Tests connectivity directly from the supplied connection parameters; the configuration does not have to be created first.

```
POST /api/n9e/ai-llm-config/test
```

### Request body

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

### Validation rules

- `api_type`, `api_url`, `api_key`, and `model` are all required

### Test behavior

Based on `api_type`, a minimal request is sent to the corresponding API ("Hi", with a maximum output of 512 tokens).

The name of the token-limit field is routed by model family: in OpenAI-compatible requests, the `gpt-5*` family (including gpt-5.1) and the `o1`/`o3`/`o4` series use `max_completion_tokens`, while other models use `max_tokens`. If the model name does not match any known family (for example a custom Azure deployment name) and the server returns a 400 saying `Use 'max_completion_tokens' instead`, the request is automatically retried once with the renamed field. In that case a `max_tokens` value set by hand in `custom_params` is migrated to `max_completion_tokens` as well, so the limit configured by the user is not lost.

Why not use a smaller value: reasoning models spend tokens on thinking first, so too small a budget leaves `content` empty and produces a spurious "no content" error. Ordinary models finish early on "Hi", so raising the limit does not increase actual consumption.

If a reasoning model spends all 512 tokens on thinking (`finish_reason=length` with an empty body), the connection is still considered healthy — the endpoint, the credentials, and the model have all been verified, which is what the probe is for. "No content" is only reported when the response finishes normally (`finish_reason=stop`) without any content at all.

| api_type | Request URL | Authentication |
|----------|---------|---------|
| openai | `{api_url}/chat/completions` | `Authorization: Bearer {api_key}` |
| claude | `{api_url}/v1/messages` | `x-api-key: {api_key}` |
| gemini | `{api_url}/v1beta/models/{model}:generateContent?key={api_key}` | URL parameter |

### Response

Success:

```json
{
  "dat": {
    "success": true,
    "duration_ms": 856
  },
  "err": ""
}
```

Failure:

```json
{
  "dat": {
    "success": false,
    "duration_ms": 5000
  },
  "err": "HTTP 401: {\"error\": \"invalid api key\"}"
}
```
