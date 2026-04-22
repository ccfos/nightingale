# PostgreSQL 查询

- **plugin_type**: `pgsql`
- **查询语言**: SQL
- **适用场景**: 指标查询

---

## 查询数据库列表

```
POST /api/n9e/db-databases
Authorization: Bearer <token>
Content-Type: application/json
Body: {"cate": "pgsql", "datasource_id": 1, "query": []}
```

## 查询表列表

```
POST /api/n9e/db-tables
Authorization: Bearer <token>
Content-Type: application/json
Body: {"cate": "pgsql", "datasource_id": 1, "query": ["database_name"]}
```

## 查询表结构

```
POST /api/n9e/db-desc-table
Authorization: Bearer <token>
Content-Type: application/json
Body: {"cate": "pgsql", "datasource_id": 1, "query": [{"database": "mydb", "table": "metrics"}]}
```

---

## 执行时序查询

```
POST /api/n9e/ds-query
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "pgsql",
  "datasource_id": 1,
  "query": [
    {
      "sql": "SELECT date_trunc('minute', created_at) AS ts, COUNT(*) AS value FROM events WHERE created_at >= to_timestamp($from) AND created_at < to_timestamp($to) GROUP BY ts ORDER BY ts",
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
| `keys.valueKey` | string | 否 | 数值列名（时序查询必填），多个用空格分隔 |
| `keys.labelKey` | string | 否 | 标签/分组列名，多个用空格分隔 |
| `keys.timeKey` | string | 否 | 时间列名 |

## 常用 SQL 示例

| 需求 | SQL |
|---|---|
| 按分钟聚合计数 | `SELECT date_trunc('minute', created_at) AS ts, COUNT(*) AS value FROM events WHERE created_at >= to_timestamp($from) AND created_at < to_timestamp($to) GROUP BY ts ORDER BY ts` |
| 按字段分组统计 | `SELECT status, COUNT(*) AS value FROM events WHERE created_at >= to_timestamp($from) AND created_at < to_timestamp($to) GROUP BY status` |
| 计算百分位 | `SELECT date_trunc('minute', created_at) AS ts, percentile_cont(0.95) WITHIN GROUP (ORDER BY response_time) AS value FROM requests WHERE created_at >= to_timestamp($from) AND created_at < to_timestamp($to) GROUP BY ts ORDER BY ts` |

## 注意事项

- **只读**：禁止 CREATE、INSERT、UPDATE、DELETE、ALTER、DROP 等写操作
- **时间变量**：`$from` 和 `$to` 为 Unix 时间戳（秒），需用 `to_timestamp()` 转换
- **时间聚合**：使用 `date_trunc('minute', col)` 做时间分桶
