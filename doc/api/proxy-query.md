# Datasource Proxy API

Nightingale 提供一个通用的反向代理接口，根据数据源 ID 将请求原样转发给后端数据源（Prometheus / VictoriaMetrics、Elasticsearch 等）。调用方无需知道后端地址、也无需维护后端的账号密码 —— 只要在 Nightingale 里配置好数据源，就可以通过 n9e 的登录态访问后端原生 API。

---

## 接口定义

```
ANY /api/n9e/proxy/:id/*url
```

- 支持所有 HTTP 方法（GET / POST / PUT / DELETE 等）。
- 需要认证（JWT 登录态或 `X-User-Token`，见下文）。
- `:id` 是数据源 ID，`*url` 是后端原生 API 的路径和查询参数。

### 认证方式

两种方式二选一：

**1. 浏览器登录态（JWT）**
前端调用时自带登录 Cookie 和 `Authorization: Bearer <access_token>`，正常登录 n9e 后无需额外配置。

**2. X-User-Token（推荐给脚本 / 外部调用）**
在个人中心生成一个固定 token，然后在请求头里带上：

```
X-User-Token: <your-token>
```

在个人中心 → 令牌管理（User Tokens）中创建 / 吊销；token 绑定到具体用户，继承该用户的权限。n9e 管理员需要在配置中开启 `http.token_auth.enable = true`，默认请求头是 `X-User-Token`，可通过 `http.token_auth.header_user_token_key` 改名。

示例：

```bash
curl -H "X-User-Token: 2f8b...c71e" \
  "http://n9e/api/n9e/proxy/1/api/v1/query?query=up"
```

### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | 数据源 ID |
| *url | string | 后端 API 的路径，会原样拼到数据源地址之后 |

### 响应

响应体、响应头、HTTP 状态码都是后端原生的结果，直接透传。

### 常见错误

| 状态码 | 响应 | 说明 |
|--------|------|------|
| 400 | `no such datasource` | 数据源不存在 |
| 400 | `invalid url path` | URL 缺少后端路径部分（如只写到 `/api/n9e/proxy/1`） |
| 500 | `invalid urls: ...` | 数据源配置的地址不合法 |
| 502 | `unauthorized access` | 后端返回了 401（通常是数据源账号密码配错） |
| 502 | 其他 | 无法连接后端、TLS 握手失败等 |

---

## 代理 Prometheus / VictoriaMetrics

Prometheus 与 VictoriaMetrics 都兼容 Prometheus HTTP API（`/api/v1/*`），通过 proxy 可直接调用。以下示例中，`1` 是 Prometheus 数据源的 ID。

### 即时查询

```bash
curl "http://n9e/api/n9e/proxy/1/api/v1/query?query=up&time=1710000000"
```

### 区间查询

```bash
curl "http://n9e/api/n9e/proxy/1/api/v1/query_range?query=up&start=1710000000&end=1710003600&step=15"
```

### 元数据

```bash
# 标签名列表
curl "http://n9e/api/n9e/proxy/1/api/v1/labels"

# 标签值
curl "http://n9e/api/n9e/proxy/1/api/v1/label/instance/values"

# Series 查询
curl "http://n9e/api/n9e/proxy/1/api/v1/series?match[]=up&start=1710000000&end=1710003600"
```

### 响应示例（`query_range`）

```json
{
  "status": "success",
  "data": {
    "resultType": "matrix",
    "result": [
      {
        "metric": { "__name__": "up", "instance": "10.0.0.1:9100" },
        "values": [[1710000000, "1"], [1710000015, "1"]]
      }
    ]
  }
}
```

### VictoriaMetrics 集群

VictoriaMetrics 集群模式通常把读路径前缀 `/select/<accountID>/prometheus` 配置在数据源地址上。调用 proxy 时只写剩余部分即可，例如：

```bash
# 数据源地址：http://vmselect:8481/select/0/prometheus
curl "http://n9e/api/n9e/proxy/2/api/v1/query?query=up"
# 最终转发到 http://vmselect:8481/select/0/prometheus/api/v1/query?query=up
```

---

## 代理 Elasticsearch

Elasticsearch 的所有原生 REST API 都可以通过 proxy 调用。以下示例中，`5` 是 Elasticsearch 数据源的 ID。

### 搜索文档

```bash
curl -X POST "http://n9e/api/n9e/proxy/5/nginx-*/_search" \
  -H 'Content-Type: application/json' \
  -d '{
    "size": 10,
    "query": {
      "bool": {
        "must": [
          { "query_string": { "query": "status:500" } },
          { "range": { "@timestamp": { "gte": "now-1h" } } }
        ]
      }
    },
    "sort": [{ "@timestamp": "desc" }]
  }'
```

### 索引元数据

```bash
# 索引列表
curl "http://n9e/api/n9e/proxy/5/_cat/indices?format=json"

# 字段 mapping
curl "http://n9e/api/n9e/proxy/5/nginx-*/_mapping/field/*"

# 集群健康
curl "http://n9e/api/n9e/proxy/5/_cluster/health"
```

### 聚合示例

```bash
curl -X POST "http://n9e/api/n9e/proxy/5/nginx-*/_search" \
  -H 'Content-Type: application/json' \
  -d '{
    "size": 0,
    "aggs": {
      "by_status": { "terms": { "field": "status", "size": 10 } }
    }
  }'
```

### 响应示例（`_search`）

```json
{
  "took": 12,
  "hits": {
    "total": { "value": 128, "relation": "eq" },
    "hits": [
      { "_index": "nginx-2024.03.01", "_id": "abc", "_source": { "...": "..." } }
    ]
  }
}
```