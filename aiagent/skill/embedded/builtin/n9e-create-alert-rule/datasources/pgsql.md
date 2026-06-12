# PostgreSQL 告警规则

- `prod`: `"metric"`
- `cate`: `"pgsql"`
- `recover_config.judge_type`: `1`（指标类型）
- **必填** `database`：查询所在数据库名
- **必填** `keys.valueKey`：SELECT 语句中数值列的别名

## 关键约束：SQL 必须用 3 段命名 `db.schema.table`

跟 MySQL 用 `db.table` 一样，PostgreSQL 也要在 SQL 里**把数据库名写进去**。

PostgreSQL 插件要求 SQL 使用 **`database.schema.table`** 三段命名格式（如 `testdb.public.events`），插件内部会：
1. 用正则从 SQL 中提取数据库名（第一段）
2. 切换连接到该数据库
3. 将三段命名格式化为 `"db"."schema"."table"` 后执行

**如果 SQL 里只写 `FROM events` 或 `FROM public.events`（缺少数据库名），会报错 `no valid table name in format database.schema.table found`**。

## OSS 版本限制

**开源版 n9e 的 PostgreSQL 数据源不支持 `$from`/`$to`/`$__timeFilter` 等时间变量**，变量不会被替换。

**正确写法**：使用 PostgreSQL 原生时间函数：
- 过去 N 分钟：`WHERE created_at >= NOW() - INTERVAL '5 minutes'`
- 过去 N 小时：`WHERE created_at >= NOW() - INTERVAL '1 hour'`
- 今天开始至今：`WHERE DATE(created_at) = CURRENT_DATE`

## triggers 硬规则（必读）

- `exp` **必填**，是告警引擎唯一评估的字段（不写 exp 的规则建出来永远不会触发，且无任何报错）
- 本数据源的变量写法：`$<ref>.<valueKey 别名>`，如 `$A.value > 5`；只有一个 valueKey 时可省略别名直接写 `$A`，多个 valueKey 时**必须带别名**（裸 `$A` 取值不确定）
- `mode` 固定填 `1`（表达式模式，前端原样展示 exp）；多条件用 `&&` / `||` 连接，如 `"$A.value > 10 && $B.value < 5"`

## rule_config 结构

```json
{
  "rule_config": {
    "queries": [
      {
        "ref": "A",
        "sql": "SELECT count(*) AS value FROM testdb.public.events WHERE created_at >= NOW() - INTERVAL '5 minutes' AND severity = 'critical'",
        "keys": {
          "valueKey": "value",
          "labelKey": ""
        },
        "interval": 60
      }
    ],
    "triggers": [
      {
        "mode": 1,
        "exp": "$A.value > 5",
        "severity": 1,
        "recover_config": {"judge_type": 1}
      }
    ]
  }
}
```

## query 字段说明

| 字段 | 必填 | 说明 |
|---|---|---|
| `ref` | ✅ | 查询引用名 |
| `sql` | ✅ | PostgreSQL SQL。**必须用 `FROM db.schema.table` 三段命名**（如 `FROM testdb.public.events`） |
| `keys.valueKey` | ✅ | **必填**，数值列的别名 |
| `keys.labelKey` | ❌ | 标签列别名，多个用空格分隔 |
| `interval` | ❌ | 查询执行间隔，**单位：总秒数**（60=1分钟，300=5分钟，3600=1小时）。**不要写 `interval_unit`** |

## 多 schema 示例

PostgreSQL 默认 schema 是 `public`，但也可能有其他 schema：

```json
{
  "ref": "A",
  "sql": "SELECT count(*) AS value FROM testdb.monitoring.events WHERE created_at >= NOW() - INTERVAL '5 minutes'",
  "keys": {"valueKey": "value"},
  "interval": 60
}
```

## 三段命名速查

| 说明 | SQL 写法 |
|---|---|
| 默认 public schema | `FROM testdb.public.events` |
| 其他 schema | `FROM testdb.monitoring.events` |
| 多表 JOIN | `FROM testdb.public.orders o JOIN testdb.public.items i ON o.id = i.order_id` |
