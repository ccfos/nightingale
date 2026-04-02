# Prometheus / VictoriaMetrics 指标查询

- **plugin_type**: `prometheus`
- **查询语言**: PromQL
- **适用场景**: 指标/时序查询

---

## 探测可用指标

获取所有指标名称列表：

```
GET /api/n9e/proxy/<datasource_id>/api/v1/label/__name__/values
Authorization: Bearer <token>
```

## 获取所有标签名

```
GET /api/n9e/proxy/<datasource_id>/api/v1/labels
Authorization: Bearer <token>
```

## 获取标签值

```
GET /api/n9e/proxy/<datasource_id>/api/v1/label/<label_name>/values
Authorization: Bearer <token>
```

## 查询时间序列元数据

```
GET /api/n9e/proxy/<datasource_id>/api/v1/series?match[]=<metric_selector>&start=<unix_ts>&end=<unix_ts>
Authorization: Bearer <token>
```

---

## 即时查询（Instant Query）

查询某一时刻的指标值：

```
POST /api/n9e/query-instant-batch
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "datasource_id": 1,
  "queries": [
    {
      "time": 1712003600,
      "query": "cpu_usage_active{ident='web-01'}"
    }
  ]
}
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `datasource_id` | int64 | 是 | 数据源 ID |
| `queries[].time` | int64 | 是 | 查询时间，Unix 时间戳（秒） |
| `queries[].query` | string | 是 | PromQL 表达式 |

**响应格式**：

```json
{
  "dat": [
    {
      "resultType": "vector",
      "result": [
        {
          "metric": {"__name__": "cpu_usage_active", "ident": "web-01"},
          "value": [1712003600, "85.6"]
        }
      ]
    }
  ]
}
```

---

## 范围查询（Range Query）

查询一段时间内的指标趋势：

```
POST /api/n9e/query-range-batch
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "datasource_id": 1,
  "queries": [
    {
      "start": 1712000000,
      "end": 1712003600,
      "step": 60,
      "query": "cpu_usage_active{ident='web-01'}"
    }
  ]
}
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `datasource_id` | int64 | 是 | 数据源 ID |
| `queries[].start` | int64 | 是 | 开始时间，Unix 时间戳（秒） |
| `queries[].end` | int64 | 是 | 结束时间，Unix 时间戳（秒） |
| `queries[].step` | int64 | 是 | 采样步长（秒） |
| `queries[].query` | string | 是 | PromQL 表达式 |

**step 建议值**：

| 时间范围 | step |
|---|---|
| 1 小时 | 15s |
| 6 小时 | 60s |
| 24 小时 | 300s |
| 7 天 | 1800s |

**响应格式**：

```json
{
  "dat": [
    {
      "resultType": "matrix",
      "result": [
        {
          "metric": {"__name__": "cpu_usage_active", "ident": "web-01"},
          "values": [
            [1712000000, "82.3"],
            [1712000060, "83.1"],
            [1712000120, "85.6"]
          ]
        }
      ]
    }
  ]
}
```

---

## 通过代理直接查询

也可通过代理直接调用 Prometheus 原生 API：

```
GET /api/n9e/proxy/<datasource_id>/api/v1/query?query=up&time=<unix_ts>
GET /api/n9e/proxy/<datasource_id>/api/v1/query_range?query=up&start=<ts>&end=<ts>&step=60
Authorization: Bearer <token>
```

---

## 常用 PromQL 示例

| 需求 | PromQL |
|---|---|
| CPU 使用率 | `cpu_usage_active` |
| 内存使用率 | `mem_used_percent` |
| 磁盘使用率 | `disk_used_percent{path="/"}` |
| 网络入流量速率 | `rate(net_bytes_recv[5m])` |
| 1分钟负载 | `system_load1` |
| HTTP 请求速率 | `rate(http_requests_total[5m])` |
| 某实例的所有指标 | `{ident="web-01"}` |
| 聚合：按实例求平均 | `avg by (ident)(cpu_usage_active)` |
| Top 10 CPU | `topk(10, cpu_usage_active)` |
