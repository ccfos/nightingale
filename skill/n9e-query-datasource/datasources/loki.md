# Loki 日志查询

- **plugin_type**: `loki`
- **查询语言**: LogQL
- **适用场景**: 日志查询

通过代理调用 Loki 原生 API。

---

## 查询日志（Range Query）

```
GET /api/n9e/proxy/<datasource_id>/loki/api/v1/query_range?query=<logql>&start=<unix_ns>&end=<unix_ns>&limit=100
Authorization: Bearer <token>
```

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `query` | string | 是 | LogQL 表达式 |
| `start` | int64 | 是 | 开始时间，纳秒级 Unix 时间戳 |
| `end` | int64 | 是 | 结束时间，纳秒级 Unix 时间戳 |
| `limit` | int | 否 | 返回日志行数，默认 100 |

**时间戳转换**：纳秒 = 秒 × 1000000000

## 即时查询

```
GET /api/n9e/proxy/<datasource_id>/loki/api/v1/query?query=<logql>&time=<unix_ns>
Authorization: Bearer <token>
```

---

## 获取标签列表

```
GET /api/n9e/proxy/<datasource_id>/loki/api/v1/labels
Authorization: Bearer <token>
```

## 获取标签值

```
GET /api/n9e/proxy/<datasource_id>/loki/api/v1/label/<label_name>/values
Authorization: Bearer <token>
```

---

## 常用 LogQL 示例

| 需求 | LogQL |
|---|---|
| 查看某应用日志 | `{job="myapp"}` |
| 搜索含 error 的日志 | `{job="myapp"} \|= "error"` |
| 排除某关键词 | `{job="myapp"} != "debug"` |
| 正则匹配 | `{job="myapp"} \|~ "timeout\|refused"` |
| 统计错误日志数量 | `count_over_time({job="myapp"} \|= "error" [5m])` |
| 按标签过滤 | `{job="myapp", level="error"}` |
| JSON 日志解析 | `{job="myapp"} \| json \| status >= 500` |
| 日志速率 | `rate({job="myapp"} \|= "error" [1m])` |

---

## 注意事项

- **时间戳为纳秒**：Loki API 的时间参数使用纳秒级 Unix 时间戳（秒 × 1000000000）
- **流选择器必填**：LogQL 查询必须包含至少一个流选择器 `{label="value"}`
- **管道操作符**：`|=`（包含）、`!=`（不包含）、`|~`（正则匹配）、`!~`（正则不匹配）
