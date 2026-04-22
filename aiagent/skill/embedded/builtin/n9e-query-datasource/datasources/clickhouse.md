# ClickHouse 查询

- **plugin_type**: `ck`
- **查询语言**: SQL
- **适用场景**: 指标/日志查询

---

## 查询数据库列表

```
POST /api/n9e/db-databases
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "ck",
  "datasource_id": 1,
  "query": []
}
```

## 查询表列表

```
POST /api/n9e/db-tables
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "ck",
  "datasource_id": 1,
  "query": ["database_name"]
}
```

## 查询表结构

```
POST /api/n9e/db-desc-table
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "ck",
  "datasource_id": 1,
  "query": [{"database": "default", "table": "logs"}]
}
```

---

## 执行时序数据查询

```
POST /api/n9e/ds-query
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "ck",
  "datasource_id": 1,
  "query": [
    {
      "ref": "A",
      "sql": "SELECT toStartOfMinute(timestamp) AS ts, avg(value) AS value FROM metrics WHERE timestamp >= $from AND timestamp < $to GROUP BY ts ORDER BY ts",
      "from": 1712000000,
      "to": 1712003600,
      "keys": {
        "valueKey": "value",
        "labelKey": "",
        "timeKey": "ts"
      }
    }
  ]
}
```

## 执行日志查询

```
POST /api/n9e/logs-query
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "ck",
  "datasource_id": 1,
  "query": [
    {
      "sql": "SELECT * FROM logs WHERE timestamp >= $from AND timestamp < $to AND message LIKE '%error%' ORDER BY timestamp DESC LIMIT 100",
      "from": 1712000000,
      "to": 1712003600
    }
  ]
}
```

---

## 查询参数

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `ref` | string | 否 | 查询引用标识，如 `"A"` |
| `sql` | string | 是 | SQL 查询语句，支持 `$from`、`$to` 时间变量 |
| `from` | int64 | 是 | 开始时间，Unix 时间戳（秒） |
| `to` | int64 | 是 | 结束时间，Unix 时间戳（秒） |
| `keys.valueKey` | string | 否 | 数值列名（时序查询必填），多个用空格分隔 |
| `keys.labelKey` | string | 否 | 标签/分组列名，多个用空格分隔 |
| `keys.timeKey` | string | 否 | 时间列名 |
| `database` | string | 否 | 数据库名 |
| `table` | string | 否 | 表名 |
| `limit` | int | 否 | 返回行数限制 |

## 时序查询响应格式

```json
{
  "dat": [
    {
      "ref": "A",
      "metric": {"host": "web-01"},
      "values": [[1712000000, 85.6], [1712000060, 83.2]],
      "query": "SELECT ..."
    }
  ]
}
```

---

## 常用 SQL 示例

| 需求 | SQL |
|---|---|
| 按分钟聚合计数 | `SELECT toStartOfMinute(ts) AS t, count() AS value FROM logs WHERE ts >= $from AND ts < $to GROUP BY t ORDER BY t` |
| 按字段分组统计 | `SELECT service, count() AS value FROM logs WHERE ts >= $from AND ts < $to GROUP BY service ORDER BY value DESC LIMIT 10` |
| 搜索日志 | `SELECT * FROM logs WHERE ts >= $from AND ts < $to AND message LIKE '%error%' ORDER BY ts DESC LIMIT 100` |

## 注意事项

- **只读**：禁止 CREATE、INSERT、UPDATE、DELETE、ALTER、DROP 等写操作
- **时间变量**：SQL 中用 `$from` 和 `$to`，系统自动替换为 Unix 时间戳（秒）
- **keys.valueKey**：时序查询必须指定，多列用空格分隔
- **时间函数**：使用 `toStartOfMinute()`、`toStartOfHour()` 等 ClickHouse 函数做时间聚合
