# AI Agent API

All endpoints require administrator privileges (`auth` + `admin`).

## Data structures

### AIAgent

| Field | Type | Required | Description |
|------|------|------|------|
| id | int64 | - | Primary key, auto-increment |
| name | string | Yes | Agent name |
| description | string | No | Description |
| use_case | string | No | Use case, e.g. `chat` |
| llm_config_id | int64 | Yes | ID of the associated LLM configuration |
| skill_ids | int64[] | No | IDs of the associated skills |
| mcp_server_ids | int64[] | No | IDs of the associated MCP servers |
| enabled | bool | No | Whether the agent is enabled; pass `true` or `false` explicitly |
| created_at | int64 | - | Creation time (Unix timestamp) |
| created_by | string | - | Creator |
| updated_at | int64 | - | Last update time (Unix timestamp) |
| updated_by | string | - | Last updated by |
| llm_config_name | string | - | Runtime field: the name of the associated LLM configuration (not stored) |

---

## List agents

```
GET /api/n9e/ai-agents
```

### Response

```json
{
  "dat": [
    {
      "id": 1,
      "name": "chat-agent",
      "description": "AI chat agent",
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

## Get agent details

```
GET /api/n9e/ai-agent/:id
```

### Path parameters

| Parameter | Type | Description |
|------|------|------|
| id | int64 | Agent ID |

### Response

```json
{
  "dat": {
    "id": 1,
    "name": "chat-agent",
    "description": "AI chat agent",
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

### Errors

- `404` the agent does not exist

---

## Create an agent

```
POST /api/n9e/ai-agents
```

### Request body

```json
{
  "name": "chat-agent",
  "description": "AI chat agent",
  "use_case": "chat",
  "llm_config_id": 1,
  "skill_ids": [1, 2],
  "mcp_server_ids": [1],
  "enabled": true
}
```

### Validation rules

- `name` is required
- `llm_config_id` is required and must be greater than 0

### Response

```json
{
  "dat": 1,
  "err": ""
}
```

Returns the ID of the newly created agent.

---

## Update an agent

```
PUT /api/n9e/ai-agent/:id
```

### Path parameters

| Parameter | Type | Description |
|------|------|------|
| id | int64 | Agent ID |

### Request body

```json
{
  "name": "chat-agent-v2",
  "description": "Updated description",
  "use_case": "chat",
  "llm_config_id": 2,
  "skill_ids": [1, 3],
  "mcp_server_ids": [],
  "enabled": true
}
```

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

- `404` the agent does not exist

---

## Delete an agent

```
DELETE /api/n9e/ai-agent/:id
```

### Path parameters

| Parameter | Type | Description |
|------|------|------|
| id | int64 | Agent ID |

### Response

```json
{
  "dat": "",
  "err": ""
}
```

### Errors

- `404` the agent does not exist

