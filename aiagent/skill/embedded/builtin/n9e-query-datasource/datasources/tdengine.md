# TDengine 查询

- **plugin_type**: `tdengine`
- **查询语言**: SQL（TDengine 方言）
- **适用场景**: 时序查询

TDengine 有独立的元数据查询端点，与通用 SQL 数据源不同。

---

## 查询数据库列表

```
POST /api/n9e/tdengine-databases
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "tdengine",
  "datasource_id": 1
}
```

## 查询表列表

```
POST /api/n9e/tdengine-tables
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "tdengine",
  "datasource_id": 1,
  "db": "power",
  "is_stable": false
}
```

| 字段 | 说明 |
|---|---|
| `db` | 数据库名 |
| `is_stable` | `false`=普通表, `true`=超级表(stable) |

## 查询表字段

```
POST /api/n9e/tdengine-columns
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "tdengine",
  "datasource_id": 1,
  "db": "power",
  "table": "meters"
}
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
  "cate": "tdengine",
  "datasource_id": 1,
  "query": [
    {
      "query": "SELECT _wstart AS ts, AVG(current) AS value FROM power.meters WHERE ts >= $from AND ts < $to INTERVAL($interval)",
      "from": "2024-04-01T00:00:00Z",
      "to": "2024-04-02T00:00:00Z",
      "interval": 60,
      "interval_unit": "s",
      "keys": {
        "metricKey": "value",
        "labelKey": "location",
        "timeFormat": ""
      }
    }
  ]
}
```

---

## 查询参数

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `query` | string | 是 | TDengine SQL 查询，支持 `$from`、`$to`、`$interval` 变量 |
| `from` | string | 是 | 开始时间，ISO 8601 格式 |
| `to` | string | 是 | 结束时间，ISO 8601 格式 |
| `interval` | int | 否 | 采样间隔数值 |
| `interval_unit` | string | 否 | 间隔单位：`s`（秒）、`m`（分）、`h`（小时） |
| `keys.metricKey` | string | 否 | 数值列名 |
| `keys.labelKey` | string | 否 | 标签/分组列名 |
| `keys.timeFormat` | string | 否 | 时间格式 |

## 时间变量

| 变量 | 说明 |
|---|---|
| `$from` | 开始时间 |
| `$to` | 结束时间 |
| `$interval` | 采样间隔，如 `60s` |

## 常用 SQL 示例

| 需求 | SQL |
|---|---|
| 按间隔聚合平均值 | `SELECT _wstart AS ts, AVG(current) AS value FROM power.meters WHERE ts >= $from AND ts < $to INTERVAL($interval)` |
| 按标签分组 | `SELECT _wstart AS ts, location, AVG(current) AS value FROM power.meters WHERE ts >= $from AND ts < $to PARTITION BY location INTERVAL($interval)` |
| 最新值 | `SELECT LAST(current) AS value, location FROM power.meters GROUP BY location` |

## 注意事项

- **专用 API**：元数据查询（databases/tables/columns）使用 `/tdengine-*` 专用端点，不走通用 `/db-*` 端点
- **超级表**：查询 stable（超级表）时设 `is_stable: true`
- **INTERVAL**：TDengine 的 `INTERVAL()` 子句用于时间窗口聚合，结合 `_wstart` 获取窗口起始时间
