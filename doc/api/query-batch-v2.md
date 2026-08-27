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

本接口同样接入仪表盘限时分享通道（`__token`），与 `/ds-query`、`/log-query-batch`
一致：带有效 board 分享 token 的匿名请求可以调用，请求里出现的每个 `datasource.id`
都必须属于该仪表盘引用的数据源集合，否则整个请求返回 HTTP 403；命中 token 后不再做
基于登录用户的 `CheckDsPerm`，并对 SQL 类数据源置位强只读校验。

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
  也可以用 `$A.<指标名>` 直接寻址某个 Ref 里的一条指标，写法与告警规则的触发条件
  一致，详见下文「表达式引用序列的方式」。
- 表达式匹配会忽略 `__name__`，因此 `$A / $B` 可以跨指标配对。同一个 Ref 内如果
  存在多条除 `__name__` 外标签完全相同的序列，它们会按 `__name__` 展开成多条结果
  序列，结果标签上会带回 `__name__` 标识是哪个指标。
- 单次表达式最多处理 10000 个标签分组和 50000 个时间点，超出时返回
  `EXPRESSION_LIMIT_EXCEEDED`，避免单个批量请求长期占用 CPU。
- 时序表达式按最终输入形状编译一次，所有分组和时间点复用同一个编译结果；求值本身
  仍与时间点数成正比，较大的时间范围或较小的 step 依然会增加 CPU 开销。
- 表达式求值过程中会检查请求是否已被取消（客户端断开、超时），已取消时对应表达式
  返回 `EXPRESSION_CANCELED` 并停止计算。
- 表达式依赖链最多 64 层，超出时返回 `EXPRESSION_DEPTH_EXCEEDED`。
- 请求级 `from/to` 是权威时间范围，会覆盖 adapter query 中已有的时间字段。写入哪些
  字段按 cate 决定：Loki / VictoriaLogs / Zabbix 写 `start`/`end`，TDengine 写
  RFC3339 的 `from`/`to`，Elasticsearch / OpenSearch 两套都写（DSL 分支读
  `start`/`end`，SQL 分支读 `from`/`to`），其余写 `from`/`to`。

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

## 表达式引用序列的方式

忽略 `__name__` 之后标签相同的序列会归入同一个匹配分组。分组内的配对规则如下。

### 裸引用 `$A`

按 `__name__` 展开：某个 Ref 在分组内有多条指标时，每条指标产出一条结果序列，
结果标签带回 `__name__`。这样 SQL 数据源一次查多列（`valueKey` 用空格分隔多个
值列，每列生成一条 `__name__` 为列名的序列）时不会互相覆盖。

分组内只有一条序列的 Ref 会广播到每一行，`$A / $B` 这种「多个指标共用一个分母」
因此照常可用。但如果这条序列自身的 `__name__` 也出现在展开维度里，它改为按指标名
精确配对，取不到对应指标的行会被跳过 —— 避免把两个无关指标配成一个无意义的值。

```jsonc
// A: 一次 SQL 查出 cpu_usage / mem_usage 两列，标签同为 host=web01
{"expression": "$A * 1"}
// 结果两条：{__name__=cpu_usage, host=web01}、{__name__=mem_usage, host=web01}
```

### 指标限定引用 `$A.<指标名>`

直接取该 Ref 里 `__name__` 等于指标名的那条序列，写法与告警规则触发条件一致。
用于同一个 Ref 内的跨指标运算 —— 这是裸引用做不到的，因为裸引用会把两条指标
展开成两行。

```jsonc
// A: 一次 SQL 查出 disk_used / disk_total 两列
{"expression": "$A.disk_used / $A.disk_total * 100"}
// 结果一条：{host=web01}，值为磁盘使用率
```

被指标限定引用过的 Ref 不参与 `__name__` 展开。分组里取不到对应指标时该分组跳过，
不报错。

限制：

- 指标限定引用要求 Ref ID 是**单个大写字母**（`$A` ～ `$Z`）。`$Q1.cpu`、`$a.cpu`
  这类会返回 `EXPRESSION_INVALID`。裸引用不受此限制。
- 指标限定引用的指标名只能由字母、数字和下划线组成。`cpu-usage` 这类含其他字符的
  列名**无法**用限定引用寻址：`$A.cpu-usage` 会在 `-` 处截断成 `$A.cpu` 加一个
  未绑定的 `usage`，返回 `EXPRESSION_EVALUATION_ERROR`（形如
  `unknown name usage`），而不是 `EXPRESSION_INVALID`。这类指标仍可通过裸引用的
  `__name__` 展开取到，展开出的结果标签会带上原样的 `__name__=cpu-usage`。
- 同一个 Ref **不能**同时用裸引用和限定引用（如 `$A + $A.cpu`），返回
  `EXPRESSION_INVALID`：被限定的 Ref 不展开，此时裸 `$A` 在多指标下没有确定含义。
- 只支持按指标名限定，不支持按标签限定（`$A.host` 这种）。
- 标量结果的 Ref 不含指标，对其使用限定引用返回 `EXPRESSION_EVALUATION_ERROR`。

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
| `EXPRESSION_CANCELED`      | 请求已取消或超时，表达式停止求值 |
| `DEPENDENCY_NOT_FOUND`     | 表达式引用未知 RefID   |
| `DEPENDENCY_TYPE_MISMATCH` | 表达式引用日志结果     |
| `DEPENDENCY_CYCLE`         | 表达式形成循环依赖     |
| `DEPENDENCY_FAILED`        | 上游依赖执行失败       |

JSON 结构、时间范围、RefID 或枚举值不合法属于整体请求错误，返回 HTTP 400。
