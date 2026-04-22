# Doris 查询

- **plugin_type**: `doris`
- **查询语言**: SQL（MySQL 兼容方言）
- **适用场景**: 日志查询

---

## 查询数据库列表

```
POST /api/n9e/db-databases
Authorization: Bearer <token>
Content-Type: application/json
Body: {"cate": "doris", "datasource_id": 1, "query": []}
```

## 查询表列表

```
POST /api/n9e/db-tables
Authorization: Bearer <token>
Content-Type: application/json
Body: {"cate": "doris", "datasource_id": 1, "query": ["database_name"]}
```

## 查询表结构

```
POST /api/n9e/db-desc-table
Authorization: Bearer <token>
Content-Type: application/json
Body: {"cate": "doris", "datasource_id": 1, "query": [{"database": "logs_db", "table": "access_log"}]}
```

---

## 执行日志查询

```
POST /api/n9e/logs-query
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "doris",
  "datasource_id": 1,
  "query": [
    {
      "sql": "SELECT * FROM logs_db.access_log WHERE log_time >= FROM_UNIXTIME($from) AND log_time < FROM_UNIXTIME($to) AND message LIKE '%error%' ORDER BY log_time DESC LIMIT 100",
      "from": 1712000000,
      "to": 1712003600,
      "database": "logs_db"
    }
  ]
}
```

## 执行时序查询

```
POST /api/n9e/ds-query
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "doris",
  "datasource_id": 1,
  "query": [
    {
      "sql": "SELECT DATE_TRUNC(log_time, INTERVAL 1 MINUTE) AS ts, COUNT(*) AS value FROM logs_db.access_log WHERE log_time >= FROM_UNIXTIME($from) AND log_time < FROM_UNIXTIME($to) GROUP BY ts ORDER BY ts",
      "from": 1712000000,
      "to": 1712003600,
      "database": "logs_db",
      "keys": {
        "valueKey": "value",
        "labelKey": "",
        "timeKey": "ts"
      }
    }
  ]
}
```

---

## 查询参数

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `sql` | string | 是 | SQL 查询语句，支持 `$from`、`$to` 时间变量 |
| `from` | int64 | 是 | 开始时间，Unix 时间戳（秒） |
| `to` | int64 | 是 | 结束时间，Unix 时间戳（秒） |
| `database` | string | 是 | 数据库名（Doris 必填） |
| `keys.valueKey` | string | 否 | 数值列名（时序查询必填） |
| `keys.labelKey` | string | 否 | 标签/分组列名 |
| `keys.timeKey` | string | 否 | 时间列名 |

## 注意事项

- **只读**：禁止写操作
- **database 必填**：Doris 查询必须指定 `database` 字段
- **时间函数**：支持 `DATE_TRUNC()`、`NOW()`、`FROM_UNIXTIME()` 等
- **MySQL 兼容**：Doris SQL 语法与 MySQL 基本兼容
