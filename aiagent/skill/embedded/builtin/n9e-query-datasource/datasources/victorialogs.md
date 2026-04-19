# VictoriaLogs 查询

- **plugin_type**: `victorialogs`
- **查询语言**: LogsQL
- **适用场景**: 日志查询

通过代理调用 VictoriaLogs 原生 API。

---

## 搜索日志

```
GET /api/n9e/proxy/<datasource_id>/select/logsql/query?query=<logsql>&start=<unix_ts>&end=<unix_ts>&limit=100
Authorization: Bearer <token>
```

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `query` | string | 是 | LogsQL 查询表达式 |
| `start` | int64 | 否 | 开始时间，Unix 时间戳（秒） |
| `end` | int64 | 否 | 结束时间，Unix 时间戳（秒） |
| `limit` | int | 否 | 返回日志行数 |

---

## 常用 LogsQL 示例

| 需求 | LogsQL |
|---|---|
| 搜索含 error 的日志 | `_msg:error` |
| 多条件 AND | `_msg:error AND service:payment` |
| 按时间流过滤 | `_stream:{job="myapp"} AND _msg:error` |
| 排除关键词 | `_msg:error NOT _msg:debug` |
| 正则匹配 | `_msg:~"timeout\|refused"` |

---

## 注意事项

- **时间戳为秒**：与 Loki 不同，VictoriaLogs 的时间参数使用秒级 Unix 时间戳
- **LogsQL 非 LogQL**：VictoriaLogs 使用 LogsQL 查询语言，语法与 Loki 的 LogQL 不同
- **_msg 字段**：默认日志内容字段为 `_msg`
- **_stream 字段**：用于按日志流过滤
