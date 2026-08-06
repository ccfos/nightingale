# V2 通用多数据源批量查询接口

Elasticsearch KQL 的支持范围与参数协议见
[`query-batch-v2-kql.md`](./query-batch-v2-kql.md)。

## Endpoint

```http
POST /api/n9e/v2/query-batch
Content-Type: application/json
```

商业版使用相同协议和接口名，路径为：

```http
POST /api/n9e-plus/v2/query-batch
```

接口支持在一次请求中查询多个数据源，并使用表达式组合时序结果。除 Prometheus 的专用
查询分支外，V2 会按请求中的 `datasource.cate` 和 `datasource.id` 从已初始化的数据源缓存
取得插件并执行。因此，新注册并完成初始化的数据源不需要额外维护 V2 类型列表；具体查询
参数及 `logs` / `time_series` 能力仍由对应数据源插件决定。

权限行为与现有查询接口一致：关闭 `AnonymousAccess.PromQuerier` 时，每个数据源查询都会
执行 `CheckDsPerm`；开启该匿名查询开关时，请求不执行数据源权限检查。该开关是全局查询
访问策略，不只影响 Prometheus 数据源。

## 请求

```json
{
  "from": 1784851200,
  "to": 1784854800,
  "queries": [
    {
      "kind": "query",
      "ref_id": "A",
      "datasource": { "cate": "prometheus", "id": 1 },
      "result_type": "time_series",
      "query": {
        "expr": "rate(http_requests_total[5m])",
        "instant": false,
        "step": 15
      }
    },
    {
      "kind": "query",
      "ref_id": "B",
      "datasource": { "cate": "iotdb", "id": 8 },
      "result_type": "time_series",
      "query": {
        "sql": "SELECT time, value, host FROM service_qps",
        "keys": {
          "valueKey": "value",
          "labelKey": "host",
          "timeKey": "time"
        }
      }
    },
    {
      "kind": "expression",
      "ref_id": "C",
      "expression": "$A / $B * 100"
    }
  ]
}
```

约束：

- `to` 必须大于等于 `from`。
- 顶层 `from/to` 只接受 Unix 时间戳秒，不接受 Unix 毫秒。
- 单次请求最多包含 100 个 query（包括数据源查询和表达式）。
- `ref_id` 必须在请求内唯一、区分大小写，并符合
  `[A-Za-z][A-Za-z0-9_]*`。
- `kind=query` 时必须提供真实的 `datasource.id`、`result_type` 和对象类型的
  `query`。`query` 使用对应 Nightingale datasource adapter 的原生结构。
- `result_type` 只能是 `time_series` 或 `logs`。
- `kind=expression` 时使用 `$A` 形式引用普通查询或其他表达式；日志不能参与表达式。
- 表达式匹配会忽略 `__name__`。同一个 Ref 内如果存在多条除 `__name__` 外标签完全
  相同的序列，它们会落入同一分组，仅最后一条序列参与计算。
- 单次表达式最多处理 10000 个标签分组和 50000 个时间点，超出时返回
  `EXPRESSION_LIMIT_EXCEEDED`，避免单个批量请求长期占用 CPU。
- 当前时序表达式会在每个匹配时间点调用一次表达式解析与计算；较大的时间范围、较小的
  step 或接近 50000 点上限的请求会增加 CPU 开销。调用方应优先缩小时间范围或增大
  step。
- 表达式依赖链最多 64 层，超出时返回 `EXPRESSION_DEPTH_EXCEEDED`。
- 请求级 `from/to` 是权威时间范围，会覆盖 adapter query 中已有的时间字段。

## 响应

接口使用 Nightingale 通用响应信封。合法的批量请求返回 HTTP 200；单项失败不会中断
其他查询，结果顺序与请求一致。

```json
{
  "dat": {
    "results": [
      {
        "ref_id": "A",
        "status": "success",
        "result_type": "time_series",
        "series": [
          {
            "labels": { "instance": "api-1" },
            "samples": [[1784851200, 12.5]]
          }
        ]
      },
      {
        "ref_id": "B",
        "status": "error",
        "error": {
          "code": "DATASOURCE_TIMEOUT",
          "message": "query timed out",
          "retryable": true
        }
      },
      {
        "ref_id": "C",
        "status": "skipped",
        "error": {
          "code": "DEPENDENCY_FAILED",
          "message": "one or more expression dependencies failed",
          "retryable": true,
          "dependency_ref_ids": ["B"]
        }
      }
    ]
  },
  "err": "",
  "request_id": "..."
}
```

时序返回 `labels` 和 `[timestamp, value]`。V2 不对 datasource adapter 返回的 sample
时间戳做二次单位换算：顶层 `from/to` 固定使用 Unix 秒；如果某数据源通过其原生查询参数
使用毫秒格式，该数据源返回的 sample 也可能是毫秒。当前 V2 请求侧只允许 Unix 秒，因此
不会由 V2 把毫秒参数传入 adapter。表达式只匹配数值完全相同的时间戳。

日志返回 `records: [{"fields": {...}}]`。空结果是 `success` 状态的空数组。

表达式沿用现有批量查询的数学运算语义：忽略 `__name__` 后标签完全相同的序列相互
匹配，只计算相同时间戳，标量可广播到序列。表达式按依赖拓扑执行；未知引用、日志
引用和循环依赖分别返回稳定错误，基础查询失败时下游表达式返回 `skipped`。

## 错误码

| code                       | 说明                   |
| -------------------------- | ---------------------- |
| `FORBIDDEN`                | 没有数据源或子资源权限 |
| `DATASOURCE_NOT_FOUND`     | 数据源不存在或未初始化 |
| `INVALID_QUERY`            | 数据源查询参数不合法   |
| `DATASOURCE_TIMEOUT`       | 查询超时或被取消       |
| `DATASOURCE_ERROR`         | 数据源执行失败         |
| `EXPRESSION_INVALID`       | 表达式语法不合法       |
| `EXPRESSION_LIMIT_EXCEEDED` | 表达式分组或点数超限  |
| `EXPRESSION_DEPTH_EXCEEDED` | 表达式依赖层数超限    |
| `EXPRESSION_EVALUATION_ERROR` | 表达式运行时求值失败 |
| `DEPENDENCY_NOT_FOUND`     | 表达式引用未知 RefID   |
| `DEPENDENCY_TYPE_MISMATCH` | 表达式引用日志结果     |
| `DEPENDENCY_CYCLE`         | 表达式形成循环依赖     |
| `DEPENDENCY_FAILED`        | 上游依赖执行失败       |

JSON 结构、时间范围、RefID 或枚举值不合法属于整体请求错误，返回 HTTP 400。
