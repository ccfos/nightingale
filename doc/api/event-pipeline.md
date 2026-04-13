# Event Pipeline API

事件 Pipeline（工作流）用于对告警事件进行自动化处理。支持线性处理器模式和工作流节点模式，可通过告警事件、API 调用或定时任务触发。

所有 `/api/n9e` 前缀的接口需要登录认证（`auth` + `user`），写操作额外需要对应权限点。`/v1/n9e` 前缀的接口为内部 Service 调用，无需用户认证。

---

## 数据结构

### EventPipeline

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int64 | - | 主键，自增 |
| name | string | 是 | Pipeline 名称，最长 128 字符 |
| typ | string | 否 | 类型：`builtin` / `user-defined` |
| use_case | string | 否 | 用途：`event_pipeline` / `alert_rule` 等 |
| trigger_mode | string | 否 | 触发模式：`event` / `api` / `cron` |
| disabled | bool | 否 | 是否禁用 |
| team_ids | int64[] | 条件必填 | 授权团队 ID 列表，`group_id=0` 时不能为空 |
| group_id | int64 | 否 | 业务组 ID。`>0` 使用业务组鉴权，`=0` 使用 team_ids 鉴权 |
| team_names | string[] | - | 团队名称（仅查询时返回） |
| description | string | 否 | 描述，最长 255 字符 |
| filter_enable | bool | 否 | 是否启用过滤 |
| label_filters | TagFilter[] | 否 | 标签过滤条件 |
| attribute_filters | TagFilter[] | 否 | 属性过滤条件 |
| processors | ProcessorConfig[] | 否 | 处理器列表（线性模式） |
| nodes | WorkflowNode[] | 否 | 工作流节点列表（工作流模式） |
| connections | Connections | 否 | 节点连接关系（工作流模式） |
| inputs | InputVariable[] | 否 | 输入参数 |
| create_at | int64 | - | 创建时间（Unix 时间戳） |
| create_by | string | - | 创建人 username |
| update_at | int64 | - | 更新时间（Unix 时间戳） |
| update_by | string | - | 更新人 username |
| update_by_nickname | string | - | 更新人昵称（仅查询时返回） |

### EventPipelineExecution

| 字段 | 类型 | 说明 |
|------|------|------|
| id | string | 执行 ID（UUID） |
| pipeline_id | int64 | Pipeline ID |
| pipeline_name | string | Pipeline 名称 |
| event_id | int64 | 关联事件 ID |
| mode | string | 触发模式：`event` / `api` / `cron` |
| status | string | 状态：`running` / `success` / `failed` |
| node_results | string | 各节点执行结果（JSON 字符串） |
| error_message | string | 错误信息 |
| error_node | string | 出错节点 ID |
| created_at | int64 | 创建时间（Unix 时间戳） |
| finished_at | int64 | 完成时间（Unix 时间戳） |
| duration_ms | int64 | 执行耗时（毫秒） |
| trigger_by | string | 触发者 username |
| inputs_snapshot | string | 输入参数快照（JSON 字符串） |

### EventPipelineExecutionStatistics

| 字段 | 类型 | 说明 |
|------|------|------|
| total | int64 | 总执行次数 |
| success | int64 | 成功次数 |
| failed | int64 | 失败次数 |
| running | int64 | 运行中数量 |
| avg_duration_ms | int64 | 平均耗时（毫秒，仅统计成功的） |
| last_run_at | int64 | 最后执行时间（Unix 时间戳） |

---

## Pipeline CRUD

### 获取 Pipeline 列表

```
GET /api/n9e/event-pipelines
```

权限点：`/event-pipelines`

#### 查询参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| group_id | int64 | 0 | 业务组 ID。`-1` 或不传：查 `group_id=0`；`0`：team_ids 鉴权模式；`>0`：指定业务组 |
| use_case | string | "" | 按用途过滤，如 `event_pipeline`、`alert_rule`。空字符串不过滤 |

#### 鉴权逻辑

- `group_id > 0`（业务组场景）：管理员返回全部；非管理员需要有该业务组权限，无权限则返回空数组
- `group_id = 0`（工作流页面场景）：管理员返回全部；非管理员只返回 `team_ids` 命中当前用户团队的条目

#### 响应

```json
{
  "dat": [
    {
      "id": 1,
      "name": "告警enrichment",
      "typ": "user-defined",
      "use_case": "event_pipeline",
      "trigger_mode": "event",
      "disabled": false,
      "team_ids": [1, 2],
      "group_id": 0,
      "team_names": ["运维组", "开发组"],
      "description": "告警事件补充标签",
      "filter_enable": true,
      "label_filters": [],
      "attribute_filters": [],
      "processors": [],
      "nodes": [],
      "connections": {},
      "inputs": [],
      "create_at": 1710000000,
      "create_by": "admin",
      "update_at": 1710000000,
      "update_by": "admin",
      "update_by_nickname": "管理员"
    }
  ],
  "err": ""
}
```

---

### 获取 Pipeline 详情

```
GET /api/n9e/event-pipeline/:id
```

权限点：`/event-pipelines`。根据 `group_id` 走业务组鉴权或 team_ids 鉴权。

#### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | Pipeline ID |

#### 响应

返回单个 EventPipeline 对象，结构同列表中的元素。

---

### 创建 Pipeline

```
POST /api/n9e/event-pipeline
```

权限点：`/event-pipelines/add`。根据请求体中的 `group_id` 走业务组鉴权（rw）或 team_ids 鉴权。

服务端自动填充 `create_by` / `update_by` 为当前用户，`create_at` / `update_at` 为当前时间。

#### 请求体

```json
{
  "name": "告警enrichment",
  "typ": "user-defined",
  "use_case": "event_pipeline",
  "trigger_mode": "event",
  "disabled": false,
  "team_ids": [1, 2],
  "group_id": 0,
  "description": "告警事件补充标签",
  "filter_enable": true,
  "label_filters": [],
  "attribute_filters": [],
  "processors": [],
  "nodes": [],
  "connections": {},
  "inputs": []
}
```

#### 校验规则

- `name` 不能为空
- `group_id <= 0` 时 `team_ids` 不能为空

#### 响应

```json
{ "dat": null, "err": "" }
```

---

### 更新 Pipeline

```
PUT /api/n9e/event-pipeline
```

权限点：`/event-pipelines/put`。根据原 Pipeline 的 `group_id` 做业务组鉴权（rw）或 team_ids 鉴权。

请求体中的 `id` 字段标识要更新的记录，服务端会保留原始的 `create_by` / `create_at`，并刷新 `update_by` / `update_at`。

#### 请求体

```json
{
  "id": 1,
  "name": "告警enrichment-v2",
  "typ": "user-defined",
  "use_case": "event_pipeline",
  "trigger_mode": "event",
  "disabled": false,
  "team_ids": [1, 2],
  "group_id": 0,
  "description": "更新后的描述",
  "filter_enable": true,
  "label_filters": [],
  "attribute_filters": [],
  "processors": [],
  "nodes": [],
  "connections": {},
  "inputs": []
}
```

#### 错误

- `404` Pipeline 不存在

---

### 批量删除 Pipeline

```
DELETE /api/n9e/event-pipelines
```

权限点：`/event-pipelines/del`。对每个待删除的 Pipeline 逐一做权限校验。

#### 请求体

```json
{
  "ids": [1, 2, 3]
}
```

#### 校验规则

- `ids` 不能为空

#### 响应

```json
{ "dat": null, "err": "" }
```

---

## Pipeline 测试执行

### 试运行 Pipeline

```
POST /api/n9e/event-pipeline-tryrun
```

权限点：`/event-pipelines`。使用工作流引擎同步执行传入的 Pipeline 配置，不持久化。

#### 请求体

```json
{
  "event_id": 12345,
  "pipeline_config": {
    "name": "test",
    "nodes": [],
    "connections": {},
    "processors": []
  },
  "input_variables": {
    "key1": "value1"
  }
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| event_id | int64 | 是 | 历史告警事件 ID，用于构造测试事件 |
| pipeline_config | EventPipeline | 是 | 完整的 Pipeline 配置 |
| input_variables | map[string]string | 否 | 输入参数覆盖 |

#### 响应

```json
{
  "dat": {
    "event": { "...": "处理后的事件对象" },
    "result": "处理结果消息",
    "status": "success",
    "node_results": []
  },
  "err": ""
}
```

当事件被丢弃时，`event` 为 `null`，`result` 为 `"event is dropped"`。

---

### 试运行单个处理器

```
POST /api/n9e/event-processor-tryrun
```

权限点：`/event-pipelines`。

#### 请求体

```json
{
  "event_id": 12345,
  "processor_config": {
    "typ": "relabel",
    "config": {}
  }
}
```

#### 响应

```json
{
  "dat": {
    "event": { "...": "处理后的事件对象" },
    "result": "处理结果消息"
  },
  "err": ""
}
```

---

### 按通知规则试运行处理器

```
POST /api/n9e/notify-rule/event-pipelines-tryrun
```

权限点：`/notification-rules/add`。按通知规则引用的 Pipeline 列表顺序执行处理器。

#### 请求体

```json
{
  "event_id": 12345,
  "pipeline_configs": [
    { "pipeline_id": 1, "enable": true },
    { "pipeline_id": 2, "enable": false }
  ]
}
```

仅 `enable=true` 的 Pipeline 会被执行。

#### 响应

```json
{
  "dat": { "...": "处理后的事件对象，或 event is dropped" },
  "err": ""
}
```

---

## API 触发执行

### 触发 Pipeline（需登录）

```
POST /api/n9e/event-pipeline/:id/trigger
```

权限点：`/event-pipelines`。异步执行，立即返回 `execution_id`。

#### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | Pipeline ID |

#### 请求体

```json
{
  "event": {
    "trigger_time": 1710000000
  },
  "inputs_overrides": {
    "key1": "value1"
  }
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| event | AlertCurEvent | 否 | 事件数据，不传则创建空事件 |
| inputs_overrides | map[string]string | 否 | 输入参数覆盖 |

#### 响应

```json
{
  "dat": {
    "execution_id": "550e8400-e29b-41d4-a716-446655440000",
    "message": "workflow execution started"
  },
  "err": ""
}
```

---

### 流式执行 Pipeline（SSE，需登录）

```
POST /api/n9e/event-pipeline/:id/stream
```

权限点：`/event-pipelines`。同步执行并以 SSE（Server-Sent Events）返回流式结果。

#### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | Pipeline ID |

#### 请求体

同 [触发 Pipeline](#触发-pipeline需登录)。

#### SSE 响应

响应头：

```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Request-ID: <request-id>
```

事件流格式：

```
data: {"type":"connected","request_id":"...","timestamp":1710000000000}

data: {"type":"thinking","content":"...","delta":"...","node_id":"node_1","done":false,"timestamp":1710000000001}

data: {"type":"tool_call","content":"...","node_id":"node_1","done":false,"timestamp":1710000000002}

data: {"type":"done","content":"...","done":true,"timestamp":1710000000003}
```

StreamChunk `type` 取值：`thinking` / `tool_call` / `tool_result` / `text` / `done` / `error`

若工作流非流式输出，则返回标准 JSON 响应。

---

## 执行记录

### 获取所有执行记录（分页）

```
GET /api/n9e/event-pipeline-executions
```

权限点：`/event-pipelines`

#### 查询参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| pipeline_id | int64 | 0 | 按 Pipeline ID 过滤，`0` 不过滤 |
| pipeline_name | string | "" | 按名称模糊搜索 |
| mode | string | "" | 按触发模式过滤：`event` / `api` / `cron` |
| status | string | "" | 按状态过滤：`running` / `success` / `failed` |
| limit | int | 20 | 每页条数（1-1000） |
| p | int | 1 | 页码 |

#### 响应

```json
{
  "dat": {
    "list": [
      {
        "id": "550e8400-e29b-41d4-a716-446655440000",
        "pipeline_id": 1,
        "pipeline_name": "告警enrichment",
        "event_id": 12345,
        "mode": "event",
        "status": "success",
        "node_results": "[...]",
        "error_message": "",
        "error_node": "",
        "created_at": 1710000000,
        "finished_at": 1710000060,
        "duration_ms": 1500,
        "trigger_by": "system"
      }
    ],
    "total": 100
  },
  "err": ""
}
```

---

### 获取指定 Pipeline 的执行记录（分页）

```
GET /api/n9e/event-pipeline/:id/executions
```

权限点：`/event-pipelines`

#### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | Pipeline ID |

#### 查询参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| mode | string | "" | 按触发模式过滤 |
| status | string | "" | 按状态过滤 |
| limit | int | 20 | 每页条数（1-1000） |
| p | int | 1 | 页码 |

#### 响应

结构同 [获取所有执行记录](#获取所有执行记录分页)。

---

### 获取执行详情

```
GET /api/n9e/event-pipeline/:id/execution/:exec_id
GET /api/n9e/event-pipeline-execution/:exec_id
```

权限点：`/event-pipelines`。两个路径等价，均通过 `exec_id` 查询。

#### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| exec_id | string | 执行 ID（UUID） |

#### 响应

返回 `EventPipelineExecution` 的全部字段，额外包含解析后的结构化数据：

```json
{
  "dat": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "pipeline_id": 1,
    "pipeline_name": "告警enrichment",
    "status": "success",
    "node_results": "[...]",
    "node_results_parsed": [
      {
        "node_id": "node_0",
        "node_name": "relabel",
        "node_type": "relabel",
        "status": "success",
        "message": "ok",
        "started_at": 1710000000,
        "finished_at": 1710000001,
        "duration_ms": 50
      }
    ],
    "inputs_snapshot_parsed": {
      "key1": "value1"
    },
    "created_at": 1710000000,
    "finished_at": 1710000060,
    "duration_ms": 1500,
    "trigger_by": "admin"
  },
  "err": ""
}
```

---

### 获取执行统计

```
GET /api/n9e/event-pipeline/:id/execution-stats
```

权限点：`/event-pipelines`

#### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | Pipeline ID |

#### 响应

```json
{
  "dat": {
    "total": 1000,
    "success": 950,
    "failed": 45,
    "running": 5,
    "avg_duration_ms": 320,
    "last_run_at": 1710000000
  },
  "err": ""
}
```

---

### 清理执行记录

```
POST /api/n9e/event-pipeline-executions/clean
```

权限点：`/event-pipelines`，需要管理员权限。

#### 请求体

```json
{
  "before_days": 30
}
```

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| before_days | int | 30 | 删除多少天前的记录，`<= 0` 时使用默认值 30 |

#### 响应

```json
{
  "dat": {
    "deleted": 1234
  },
  "err": ""
}
```

---

## 内部 Service 接口

以下接口供内部服务（如 edge 节点）调用，路径前缀为 `/v1/n9e`，无需用户认证。

### 获取全量 Pipeline 列表

```
GET /v1/n9e/event-pipelines
```

返回所有 Pipeline（不做 group_id / use_case 过滤、不做权限过滤）。

#### 响应

```json
{
  "dat": [ { "...": "EventPipeline 对象" } ],
  "err": ""
}
```

---

### Service 触发 Pipeline

```
POST /v1/n9e/event-pipeline/:id/trigger
```

异步执行，立即返回 `execution_id`。

#### 请求体

```json
{
  "event": null,
  "inputs_overrides": {},
  "username": "system"
}
```

`username` 用于标识触发者。

#### 响应

```json
{
  "dat": {
    "execution_id": "550e8400-e29b-41d4-a716-446655440000",
    "message": "workflow execution started"
  },
  "err": ""
}
```

---

### Service 流式执行 Pipeline

```
POST /v1/n9e/event-pipeline/:id/stream
```

同步执行并以 SSE 返回流式结果。请求体和 SSE 格式同 [流式执行 Pipeline](#流式执行-pipelinesse需登录)，区别是触发者从 `username` 字段取值而非从登录态获取。

---

### 同步执行记录（Edge → Center）

```
POST /v1/n9e/event-pipeline-execution
```

Edge 节点将本地执行记录同步到 Center。

#### 请求体

完整的 `EventPipelineExecution` 对象。

#### 校验规则

- `id` 不能为空
- `pipeline_id` 必须 > 0

#### 响应

```json
{ "dat": null, "err": "" }
```
