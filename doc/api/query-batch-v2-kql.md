# Elasticsearch KQL 查询支持

> 状态：已实现第一阶段 KQL 支持。
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
| `filter` | string | `filter_language=kql` 时是 | KQL 表达式 |
| `kql_options.default_field` | string | 否 | 未指定字段的裸词默认查询字段 |
| `kql_options.case_insensitive` | boolean | 否 | `wildcard` / `prefix` 是否忽略大小写，默认 `false` |
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
| 字段匹配 | `match`（`operator: and`） | `status: 200`、`message: timeout error` |
| 精确短语 | `match_phrase` | `message: "timeout error"` |
| 字段存在 | `exists` | `trace.id: *` |
| 后缀通配 | `prefix` | `service: api*` |
| 其他 `*` 通配 | `wildcard` | `message: *timeout*` |
| 范围比较 | `range` | `bytes >= 1024`、`@timestamp < now-2d` |
| 布尔运算 | `bool` | `a: 1 AND b: 2`、`a: 1 b: 2`、`NOT status: 200` |
| 括号与同字段多值 | `bool.should` | `status: (200 OR 201)` |

`AND`、`OR`、`NOT` 不区分大小写。相邻的字段条件按隐式 `AND` 处理，例如
`status: 200 level: ERROR` 等价于 `status: 200 AND level: ERROR`。可使用括号明确优先级。
双引号内的 `*` 是普通字符，例如 `message: "foo*"` 会按短语匹配，不会生成通配查询。
字段后的多个未加引号词会合并为一个 `match` 查询，例如 `message: timeout error`；
如需表达多个候选值，应写成 `status: (200 OR 201)`。
当前编译器不读取 Elasticsearch mapping，因此不会像 Kibana 一样根据 `text` / `keyword`
字段类型切换查询类型；未加引号且不含通配符的值统一生成 `match`。

## 当前限制

以下语法目前会返回单项 `INVALID_QUERY`，不会降级为 Lucene：

- nested 作用域，例如 `user:{ first: Alice AND last: White }`；
- 字段名通配符，例如 `datastream.*: logs`；
- 括号内未使用布尔操作符的多词值，例如 `message: (timeout error)`；请改成
  `message: timeout error`，或显式写成 `message: (timeout AND error)`；
- Lucene regexp、fuzzy、proximity、boost 等非 KQL 语法；
- 未加引号的 `/` 字符。URL 等值请使用引号，例如
  `http.request.referrer: "https://example.com"`。

前导 `*` 通配可能造成全索引扫描，前端应在输入时给出性能提示。
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
