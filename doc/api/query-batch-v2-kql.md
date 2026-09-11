# Elasticsearch KQL 查询支持

> 状态：与前端 `buildESQueryFromKuery` 的默认转换行为对齐。
>
> 本文仅说明 V2 批量查询中 Elasticsearch 的 KQL 扩展；通用 V2 请求、响应与错误码见
> [`query-batch-v2.md`](./query-batch-v2.md)。

## 使用方式

在 `datasource.cate = "elasticsearch"` 的 query 对象中传入：

```json
{
  "filter_language": "kql",
  "filter": "service.name: api AND log.level: \"ERROR\" AND http.response.status_code >= 500",
  "kql_options": {
    "case_insensitive": false,
    "time_zone": "Asia/Shanghai"
  }
}
```

新增字段：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `filter_language` | string | 否 | 传 `kql` 启用 KQL；未传或 `lucene` 保持旧的 Lucene 行为 |
| `filter` | string | 否 | KQL 表达式；留空与 Lucene 一致，表示只按时间范围查全部 |
| `kql_options.default_field` | string | 否 | **已忽略**，仅为请求兼容保留；裸词按前端行为查询全字段 |
| `kql_options.case_insensitive` | boolean | 否 | **已忽略**，仅为请求兼容保留；值通配生成 `query_string`，大小写行为由字段的分析器决定 |
| `kql_options.time_zone` | string | 否 | 仅对本请求 `date_field` 的范围条件生效；用于日期文字解释时区，如 `Asia/Shanghai` 或 `+08:00` |

顶层 V2 的 `from/to` 仍是权威时间范围；KQL 中的时间条件会与其共同作为 filter 生效。

## 编译方式

KQL 不会直接传给 Lucene `query_string`。后端将 KQL 解析为 AST，再编译为标准
Elasticsearch Query DSL，最后与 V2 时间范围合并至 `bool.filter`。

这样可避免 KQL 与 Lucene 在范围比较、字段存在、转义和布尔语义上的差异。项目支持 ES
7.10 及以上版本，因此不能把仅较新 ES 才提供的原生 `kql` Query DSL 作为通用依赖。

## 已支持语法

| KQL | 生成的 DSL | 示例 |
| --- | --- | --- |
| 字段匹配 | `match` | `status: 200`、`message: timeout error` |
| 精确短语 | `match_phrase` | `message: "timeout error"` |
| 字段存在 | `exists` | `trace.id: *` |
| 值通配 | `query_string` | `service: api*` |
| 字段名通配 | 与前端默认模式相同的字段名 DSL | `datastream.*: logs` |
| 范围比较 | `range` | `bytes >= 1024`、`@timestamp < now-2d` |
| 布尔运算 | `bool` | `a: 1 AND b: 2`、`NOT status: 200` |
| 括号与同字段多值 | `bool.should` | `status: (200 OR 201)` |
| nested 作用域 | `nested` | `user:{ first: Alice AND last: White }` |
| 全量匹配 | `match_all` | `*:*` |

`AND`、`OR`、`NOT` 不区分大小写，但字段条件之间必须显式使用 `AND` 或 `OR`。
可使用括号明确优先级；字段后的括号可以容纳多词值，例如
`message: (timeout error)`。未加引号的多词值中的空格是值的一部分，其中可以带通配符，
例如 `message: foo bar*` 会整体作为一个通配值下发。双引号内的 `*` 是普通字符，
例如 `message: "foo*"` 会按短语匹配。未加引号的 `/`、`~`、`^`、`[]` 是普通值内容；
支持反斜杠转义与 `\t`、`\r`、`\n`、`\uXXXX`。

当前编译器不读取 Elasticsearch mapping，因而严格复现前端默认转换器的无 mapping 分支，
不会按 `text` / `keyword` 字段类型切换查询类型。

未加引号的值除 `true` / `false` / `null` 外一律按字符串下发，与前端文法一致，
例如 `bytes >= 1024` 生成 `{"gte": "1024"}`；数值与日期（含 epoch 毫秒）字段由
Elasticsearch 按 mapping 解析。

字段名通配符（包括 `*: value` 与 `*prefix: value`）会按前端默认转换器原样传入 DSL，
不会受值通配的前导 `*` 限制。范围值中的通配符同样会原样传递给 `range`；这可能因字段类型
而被 Elasticsearch 拒绝或得到非预期结果，建议仅在确有兼容需求时使用。

## 当前限制

非单独 `*` 的前导通配默认返回 `INVALID_QUERY`，例如 `message: *timeout` 和 `*timeout`。
这与前端导出函数的 `allowLeadingWildcards=false` 默认值一致；`field: *` 与 `*:*` 不受此限制。
KQL 的括号和连续 `NOT` 嵌套最多 64 层。

## 示例

```json
{
  "from": 1784971200,
  "to": 1784974800,
  "queries": [
    {
      "kind": "query",
      "ref_id": "ES_LOGS",
      "datasource": {"cate": "elasticsearch", "id": 7},
      "result_type": "logs",
      "query": {
        "index_type": "index",
        "index": "logs-*",
        "date_field": "@timestamp",
        "limit": 100,
        "ascending": false,
        "filter_language": "kql",
        "filter": "service.name: api AND log.level: ERROR AND http.response.status_code >= 500",
        "kql_options": {"time_zone": "Asia/Shanghai"}
      }
    }
  ]
}
```

KQL 查询成功后的日志记录遵循 V2 通用响应；Elasticsearch/OpenSearch 命中的 `_source` 会直接成为
`records[].fields`，不会返回 `_id`、`_index` 或 `sort`。

## 参考

- [Elastic：KQL 语法](https://www.elastic.co/docs/reference/query-languages/kql)
- [Elastic：Lucene query_string 语法](https://www.elastic.co/docs/reference/query-languages/query-dsl/query-dsl-query-string-query)
