# Event Pipeline API

Event pipelines (workflows) automate the processing of alert events. They support both a linear processor mode and a workflow node mode, and can be triggered by alert events, API calls, or scheduled jobs.

Every endpoint under the `/api/n9e` prefix requires login authentication (`auth` + `user`), and write operations additionally require the corresponding permission point. Endpoints under the `/v1/n9e` prefix are for internal service calls and require no user authentication.

---

## Data structures

### EventPipeline

| Field | Type | Required | Description |
|------|------|------|------|
| id | int64 | - | Primary key, auto-increment |
| name | string | Yes | Pipeline name, at most 128 characters |
| typ | string | No | Type: `builtin` / `user-defined` |
| use_case | string | No | Use case: `event_pipeline` / `alert_rule`, etc. |
| trigger_mode | string | No | Trigger mode: `event` / `api` / `cron` |
| disabled | bool | No | Whether the pipeline is disabled |
| team_ids | int64[] | Conditional | IDs of the authorized teams; must not be empty when `group_id=0` |
| group_id | int64 | No | Business group ID. `>0` uses business group authorization, `=0` uses team_ids authorization |
| team_names | string[] | - | Team names (returned on read only) |
| description | string | No | Description, at most 255 characters |
| filter_enable | bool | No | Whether filtering is enabled |
| label_filters | TagFilter[] | No | Label filter conditions |
| attribute_filters | TagFilter[] | No | Attribute filter conditions |
| processors | ProcessorConfig[] | No | Processor list (linear mode) |
| nodes | WorkflowNode[] | No | Workflow node list (workflow mode) |
| connections | Connections | No | Node connections (workflow mode) |
| inputs | InputVariable[] | No | Input parameters |
| create_at | int64 | - | Creation time (Unix timestamp) |
| create_by | string | - | Username of the creator |
| update_at | int64 | - | Last update time (Unix timestamp) |
| update_by | string | - | Username of the last updater |
| update_by_nickname | string | - | Nickname of the last updater (returned on read only) |

### EventPipelineExecution

| Field | Type | Description |
|------|------|------|
| id | string | Execution ID (UUID) |
| pipeline_id | int64 | Pipeline ID |
| pipeline_name | string | Pipeline name |
| event_id | int64 | ID of the associated event |
| mode | string | Trigger mode: `event` / `api` / `cron` |
| status | string | Status: `running` / `success` / `failed` |
| node_results | string | Per-node execution results (JSON string) |
| error_message | string | Error message |
| error_node | string | ID of the node that failed |
| created_at | int64 | Creation time (Unix timestamp) |
| finished_at | int64 | Completion time (Unix timestamp) |
| duration_ms | int64 | Execution time (milliseconds) |
| trigger_by | string | Username of the trigger |
| inputs_snapshot | string | Snapshot of the input parameters (JSON string) |

### EventPipelineExecutionStatistics

| Field | Type | Description |
|------|------|------|
| total | int64 | Total number of executions |
| success | int64 | Number of successes |
| failed | int64 | Number of failures |
| running | int64 | Number currently running |
| avg_duration_ms | int64 | Average duration in milliseconds (successful executions only) |
| last_run_at | int64 | Time of the most recent execution (Unix timestamp) |

---

## Pipeline CRUD

### List pipelines

```
GET /api/n9e/event-pipelines
```

Permission point: `/event-pipelines`

#### Query parameters

| Parameter | Type | Default | Description |
|------|------|--------|------|
| group_id | int64 | 0 | Business group ID. `-1` or omitted: query `group_id=0`; `0`: team_ids authorization mode; `>0`: the specified business group |
| use_case | string | "" | Filter by use case, e.g. `event_pipeline` or `alert_rule`. An empty string means no filtering |

#### Authorization logic

- `group_id > 0` (business group scenario): administrators get everything; non-administrators need permission on that business group, and without it an empty array is returned
- `group_id = 0` (workflow page scenario): administrators get everything; non-administrators only get entries whose `team_ids` match one of the current user's teams

#### Response

```json
{
  "dat": [
    {
      "id": 1,
      "name": "alert-enrichment",
      "typ": "user-defined",
      "use_case": "event_pipeline",
      "trigger_mode": "event",
      "disabled": false,
      "team_ids": [1, 2],
      "group_id": 0,
      "team_names": ["Ops Team", "Dev Team"],
      "description": "Enrich alert events with extra labels",
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
      "update_by_nickname": "Administrator"
    }
  ],
  "err": ""
}
```

---

### Get pipeline details

```
GET /api/n9e/event-pipeline/:id
```

Permission point: `/event-pipelines`. Depending on `group_id`, either business group authorization or team_ids authorization applies.

#### Path parameters

| Parameter | Type | Description |
|------|------|------|
| id | int64 | Pipeline ID |

#### Response

Returns a single EventPipeline object with the same structure as an element of the list.

---

### Create a pipeline

```
POST /api/n9e/event-pipeline
```

Permission point: `/event-pipelines/add`. Depending on the `group_id` in the request body, either business group authorization (rw) or team_ids authorization applies.

The server fills in `create_by` / `update_by` with the current user and `create_at` / `update_at` with the current time.

#### Request body

```json
{
  "name": "alert-enrichment",
  "typ": "user-defined",
  "use_case": "event_pipeline",
  "trigger_mode": "event",
  "disabled": false,
  "team_ids": [1, 2],
  "group_id": 0,
  "description": "Enrich alert events with extra labels",
  "filter_enable": true,
  "label_filters": [],
  "attribute_filters": [],
  "processors": [],
  "nodes": [],
  "connections": {},
  "inputs": []
}
```

#### Validation rules

- `name` must not be empty
- `team_ids` must not be empty when `group_id <= 0` and `use_case != "alert_rule"`

#### Response

```json
{ "dat": 123, "err": "" }
```

`dat` is the ID of the newly created pipeline.

---

### Update a pipeline

```
PUT /api/n9e/event-pipeline
```

Permission point: `/event-pipelines/put`. Depending on the original pipeline's `group_id`, either business group authorization (rw) or team_ids authorization applies.

The `id` field in the request body identifies the record to update. The server preserves the original `create_by` / `create_at` and refreshes `update_by` / `update_at`.

#### Request body

```json
{
  "id": 1,
  "name": "alert-enrichment-v2",
  "typ": "user-defined",
  "use_case": "event_pipeline",
  "trigger_mode": "event",
  "disabled": false,
  "team_ids": [1, 2],
  "group_id": 0,
  "description": "Updated description",
  "filter_enable": true,
  "label_filters": [],
  "attribute_filters": [],
  "processors": [],
  "nodes": [],
  "connections": {},
  "inputs": []
}
```

#### Errors

- `404` the pipeline does not exist

---

### Batch delete pipelines

```
DELETE /api/n9e/event-pipelines
```

Permission point: `/event-pipelines/del`. Permissions are checked for each pipeline to be deleted.

#### Request body

```json
{
  "ids": [1, 2, 3]
}
```

#### Validation rules

- `ids` must not be empty

#### Response

```json
{ "dat": null, "err": "" }
```

---

## Pipeline test execution

### Dry-run a pipeline

```
POST /api/n9e/event-pipeline-tryrun
```

Permission point: `/event-pipelines`. Runs the supplied pipeline configuration synchronously through the workflow engine without persisting anything.

#### Request body

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

| Field | Type | Required | Description |
|------|------|------|------|
| event_id | int64 | Yes | ID of a historical alert event, used to build the test event |
| pipeline_config | EventPipeline | Yes | The complete pipeline configuration |
| input_variables | map[string]string | No | Overrides for the input parameters |

#### Response

```json
{
  "dat": {
    "event": { "...": "the processed event object" },
    "result": "processing result message",
    "status": "success",
    "node_results": []
  },
  "err": ""
}
```

When the event is dropped, `event` is `null` and `result` is `"event is dropped"`.

---

### Dry-run a single processor

```
POST /api/n9e/event-processor-tryrun
```

Permission point: `/event-pipelines`.

#### Request body

```json
{
  "event_id": 12345,
  "processor_config": {
    "typ": "relabel",
    "config": {}
  }
}
```

#### Response

```json
{
  "dat": {
    "event": { "...": "the processed event object" },
    "result": "processing result message"
  },
  "err": ""
}
```

---

### Dry-run processors for a notification rule

```
POST /api/n9e/notify-rule/event-pipelines-tryrun
```

Permission point: `/notification-rules/add`. Runs the processors in the order of the pipeline list referenced by the notification rule.

#### Request body

```json
{
  "event_id": 12345,
  "pipeline_configs": [
    { "pipeline_id": 1, "enable": true },
    { "pipeline_id": 2, "enable": false }
  ]
}
```

Only pipelines with `enable=true` are executed.

#### Response

```json
{
  "dat": { "...": "the processed event object, or event is dropped" },
  "err": ""
}
```

---

## API-triggered execution

### Trigger a pipeline (requires login)

```
POST /api/n9e/event-pipeline/:id/trigger
```

Permission point: `/event-pipelines`. Runs asynchronously and returns an `execution_id` immediately.

#### Path parameters

| Parameter | Type | Description |
|------|------|------|
| id | int64 | Pipeline ID |

#### Request body

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

| Field | Type | Required | Description |
|------|------|------|------|
| event | AlertCurEvent | No | Event data; when omitted an empty event is created |
| inputs_overrides | map[string]string | No | Overrides for the input parameters |

#### Response

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

### Stream pipeline execution (SSE, requires login)

```
POST /api/n9e/event-pipeline/:id/stream
```

Permission point: `/event-pipelines`. Runs synchronously and returns the result as an SSE (Server-Sent Events) stream.

#### Path parameters

| Parameter | Type | Description |
|------|------|------|
| id | int64 | Pipeline ID |

#### Request body

Same as [Trigger a pipeline](#trigger-a-pipeline-requires-login).

#### SSE response

Response headers:

```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Request-ID: <request-id>
```

Event stream format:

```
data: {"type":"connected","request_id":"...","timestamp":1710000000000}

data: {"type":"thinking","content":"...","delta":"...","node_id":"node_1","done":false,"timestamp":1710000000001}

data: {"type":"tool_call","content":"...","node_id":"node_1","done":false,"timestamp":1710000000002}

data: {"type":"done","content":"...","done":true,"timestamp":1710000000003}
```

Possible values of the StreamChunk `type`: `thinking` / `tool_call` / `tool_result` / `text` / `done` / `error`

If the workflow does not produce streaming output, a standard JSON response is returned instead.

---

## Execution records

### List all execution records (paginated)

```
GET /api/n9e/event-pipeline-executions
```

Permission point: `/event-pipelines`

#### Query parameters

| Parameter | Type | Default | Description |
|------|------|--------|------|
| pipeline_id | int64 | 0 | Filter by pipeline ID; `0` means no filtering |
| pipeline_name | string | "" | Fuzzy search by name |
| mode | string | "" | Filter by trigger mode: `event` / `api` / `cron` |
| status | string | "" | Filter by status: `running` / `success` / `failed` |
| limit | int | 20 | Page size (1-1000) |
| p | int | 1 | Page number |

#### Response

```json
{
  "dat": {
    "list": [
      {
        "id": "550e8400-e29b-41d4-a716-446655440000",
        "pipeline_id": 1,
        "pipeline_name": "alert-enrichment",
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

### List execution records for a pipeline (paginated)

```
GET /api/n9e/event-pipeline/:id/executions
```

Permission point: `/event-pipelines`

#### Path parameters

| Parameter | Type | Description |
|------|------|------|
| id | int64 | Pipeline ID |

#### Query parameters

| Parameter | Type | Default | Description |
|------|------|--------|------|
| mode | string | "" | Filter by trigger mode |
| status | string | "" | Filter by status |
| limit | int | 20 | Page size (1-1000) |
| p | int | 1 | Page number |

#### Response

Same structure as [List all execution records](#list-all-execution-records-paginated).

---

### Get execution details

```
GET /api/n9e/event-pipeline/:id/execution/:exec_id
GET /api/n9e/event-pipeline-execution/:exec_id
```

Permission point: `/event-pipelines`. The two paths are equivalent; both look the record up by `exec_id`.

#### Path parameters

| Parameter | Type | Description |
|------|------|------|
| exec_id | string | Execution ID (UUID) |

#### Response

Returns every field of `EventPipelineExecution`, plus the parsed structured data:

```json
{
  "dat": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "pipeline_id": 1,
    "pipeline_name": "alert-enrichment",
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

### Get execution statistics

```
GET /api/n9e/event-pipeline/:id/execution-stats
```

Permission point: `/event-pipelines`

#### Path parameters

| Parameter | Type | Description |
|------|------|------|
| id | int64 | Pipeline ID |

#### Response

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

### Clean up execution records

```
POST /api/n9e/event-pipeline-executions/clean
```

Permission point: `/event-pipelines`; administrator privileges are required.

#### Request body

```json
{
  "before_days": 30
}
```

| Field | Type | Default | Description |
|------|------|--------|------|
| before_days | int | 30 | Delete records older than this many days; values `<= 0` fall back to the default of 30 |

#### Response

```json
{
  "dat": {
    "deleted": 1234
  },
  "err": ""
}
```

---

## Internal service endpoints

The following endpoints are for internal services (such as edge nodes). They are prefixed with `/v1/n9e` and require no user authentication.

### List all pipelines

```
GET /v1/n9e/event-pipelines
```

Returns every pipeline (no group_id / use_case filtering and no permission filtering).

#### Response

```json
{
  "dat": [ { "...": "EventPipeline object" } ],
  "err": ""
}
```

---

### Service-triggered pipeline execution

```
POST /v1/n9e/event-pipeline/:id/trigger
```

Runs asynchronously and returns an `execution_id` immediately.

#### Request body

```json
{
  "event": null,
  "inputs_overrides": {},
  "username": "system"
}
```

`username` identifies the trigger.

#### Response

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

### Service-triggered streaming pipeline execution

```
POST /v1/n9e/event-pipeline/:id/stream
```

Runs synchronously and returns the result as an SSE stream. The request body and SSE format are the same as for [Stream pipeline execution](#stream-pipeline-execution-sse-requires-login); the difference is that the trigger is taken from the `username` field instead of from the login session.

---

### Sync an execution record (Edge → Center)

```
POST /v1/n9e/event-pipeline-execution
```

An edge node syncs a locally produced execution record to center.

#### Request body

The complete `EventPipelineExecution` object.

#### Validation rules

- `id` must not be empty
- `pipeline_id` must be > 0

#### Response

```json
{ "dat": null, "err": "" }
```
