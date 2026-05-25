# Pushgw Proxy Remote Write API

Nightingale pushgw 提供一个**纯转发**的 remote_write 接口：客户端把 Prometheus remote_write 数据推给 pushgw，pushgw 不解析 body、不进队列，直接将原始字节按配置文件中的 `Writers` 列表逐个转发给后端（Prometheus / VictoriaMetrics / Mimir / 其他兼容 remote_write 协议的存储）。

相比 `/prometheus/v1/write` 走内存队列 + relabel + sharding 的完整链路，`/proxy/v1/write` 更接近一个"带认证和并发保护的 L7 反向代理"，适用于：

- pushgw 本身只想做一层接入网关，写入聚合交给后端集群；
- 多机房 / 多副本 fan-out，需要把同一份数据复制到多个后端；
- 想要原样保留客户端 header（`Content-Encoding`、`X-Prometheus-Remote-Write-Version` 等），不希望被 pushgw 重新打包。

---

## 接口定义

```
POST /proxy/v1/write
```

- 请求 body：标准 Prometheus remote_write，即 protobuf + snappy 压缩。
- pushgw 不会解析 body，原样转发；query string 也会原样附加到每个 writer 的 URL 上。
- 是否需要认证由 pushgw 配置决定（见下文 `BasicAuth`）。

### 认证方式

是否启用认证由 `pushgw.yaml` 中的 `HTTP.APIForAgent.BasicAuth` / `HTTP.APIForService.BasicAuth` 决定：

- 任一 map 非空 → 启用 HTTP Basic Auth，需在请求头携带 `Authorization: Basic <base64(user:pass)>`；
- 两者均为空 → 不需要认证。

示例：

```bash
curl -u myuser:mypass \
  -H 'Content-Type: application/x-protobuf' \
  -H 'Content-Encoding: snappy' \
  -H 'X-Prometheus-Remote-Write-Version: 0.1.0' \
  --data-binary @payload.snappy \
  "http://pushgw:17000/proxy/v1/write"
```

### 请求头透传规则

下列请求头会被 pushgw 透传到后端 writer；缺省时使用默认值：

| 请求头 | 默认值 | 说明 |
|--------|--------|------|
| `Content-Type` | `application/x-protobuf` | remote_write 标准 |
| `Content-Encoding` | `snappy` | remote_write 标准 |
| `User-Agent` | `n9e` | 如果客户端带了，会被追加 `-n9e` 后缀（例如 `prometheus/2.45.0-n9e`） |
| `X-Prometheus-Remote-Write-Version` | `0.1.0` | remote_write 协议版本 |

其他请求头不会主动透传。后端 writer 自身的 `BasicAuthUser` / `BasicAuthPass` / `Headers` 在转发时单独设置（在配置文件里配，详见下文）。

### Query String 透传

请求 URL 的 query string 会原样拼接到每个 writer 的 URL 后面。例如：

- writer 配置：`http://vminsert:8480/insert/0/prometheus/api/v1/write`
- 客户端请求：`POST /proxy/v1/write?extra_label=cluster%3Dcn-bj`
- 实际转发：`POST http://vminsert:8480/insert/0/prometheus/api/v1/write?extra_label=cluster%3Dcn-bj`

若 writer URL 本身已带 `?`，则用 `&` 续接。

---

## 响应

### 成功

```
HTTP/1.1 200 OK
```

注意：**只要 pushgw 收到并准备好转发，就立即返回 200**。后端 writer 是否成功（含 4xx/5xx、超时、连接失败）只会反映在日志和 metrics 中，**不会回写到客户端响应**。这是 fan-out + 多 writer 设计的必然结果——任意一个 writer 慢/挂都不应该影响整个请求。

### 失败

| 状态码 | 响应 | 触发条件 |
|--------|------|----------|
| 400 | `{"error": "..."}` | 读取 body 失败（连接中断、客户端关闭等） |
| 413 | `proxy remote write body too large: > <N> bytes` | 单个请求 body 超过 `ProxyMaxBodyBytes` |
| 429 | `proxy remote write inflight over limit: <N>` | 并发 in-flight 数超过 `ProxyInflightMax` |

429 是**背压**信号，配合 remote_write 客户端原生的 WAL + 退避重试机制，客户端会自动重试，无需上层处理。

---

## 背压与限流

`/proxy/v1/write` 通过两个全局参数控制内存上限：

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `Pushgw.ProxyInflightMax` | `1000` | 单个 pushgw 进程的并发上限。超过直接 429，请求**不计入** writer 转发 |
| `Pushgw.ProxyMaxBodyBytes` | `32 * 1024 * 1024`（32 MiB） | 单个请求 body 最大字节数，超过返回 413 |

`pushgw.yaml` 示例：

```yaml
Pushgw:
  ProxyInflightMax: 2000
  ProxyMaxBodyBytes: 67108864   # 64 MiB

  Writers:
    - Url: http://victoriametrics-1:8428/api/v1/write
      Timeout: 10000
      DialTimeout: 3000
      MaxIdleConns: 100
      MaxIdleConnsPerHost: 100
      IdleConnTimeout: 90000
      Headers:
        - X-Scope-OrgID
        - n9e
    - Url: http://victoriametrics-2:8428/api/v1/write
      BasicAuthUser: writer
      BasicAuthPass: secret
      Timeout: 10000
```

> 内存占用上限约为 `ProxyInflightMax × ProxyMaxBodyBytes`，按 1000 × 32 MiB ≈ 32 GiB 估算，实际峰值远小于此（多数 remote_write 批次在 64–256 KiB）。

### Writers 配置说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `Url` | string | 后端 remote_write 地址（必填） |
| `BasicAuthUser` / `BasicAuthPass` | string | 后端基本认证账号 |
| `Timeout` | int (ms) | 整个请求超时 |
| `DialTimeout` | int (ms) | TCP 建连超时 |
| `MaxConnsPerHost` / `MaxIdleConns` / `MaxIdleConnsPerHost` | int | HTTP 连接池参数 |
| `IdleConnTimeout` / `KeepAlive` / `TLSHandshakeTimeout` / `ExpectContinueTimeout` | int (ms) | HTTP 传输层各类超时 |
| `Headers` | []string | 自定义请求头，按 `[key1, val1, key2, val2, ...]` 成对写入。若 key 为 `Host`，会同时设置 `req.Host` |

> 注意：`/proxy/v1/write` 不使用 `WriteRelabels`（不解析 body 自然没法 relabel）。relabel 只在 `/prometheus/v1/write` 路径上生效。

---

## 监控指标

pushgw 在 `/metrics` 接口暴露以下指标（namespace `n9e_pushgw`）：

| 指标 | 类型 | 标签 | 说明 |
|------|------|------|------|
| `n9e_pushgw_proxy_remote_write_total` | Counter | - | 收到的 `/proxy/v1/write` 请求总数 |
| `n9e_pushgw_proxy_remote_write_inflight` | Gauge | - | 当前 in-flight 请求数（背压观测的核心指标） |
| `n9e_pushgw_proxy_remote_write_over_limit_total` | Counter | - | 因 in-flight 超限被 429 拒绝的请求数 |
| `n9e_pushgw_proxy_remote_write_body_too_large_total` | Counter | - | 因 body 超限被 413 拒绝的请求数 |
| `n9e_pushgw_proxy_forward_total` | Counter | `url` | 向各 writer 发起的转发次数 |
| `n9e_pushgw_proxy_forward_error_total` | Counter | `url`, `reason` | 转发失败次数。`reason` 取值：`build_request` / `do_request` / `status_4xx_5xx` |
| `n9e_pushgw_proxy_forward_duration_seconds` | Histogram | `url` | 单次转发耗时分布 |

推荐告警：

- `n9e_pushgw_proxy_remote_write_inflight` 长期接近 `ProxyInflightMax` → 扩容或调高阈值；
- `rate(n9e_pushgw_proxy_remote_write_over_limit_total[5m]) > 0` 持续触发 → 后端写入慢，客户端可能丢数据；
- `rate(n9e_pushgw_proxy_forward_error_total[5m]) > 0` 按 `url` / `reason` 切片排障。

---

## 客户端配置示例

### Prometheus

```yaml
remote_write:
  - url: http://pushgw:17000/proxy/v1/write
    basic_auth:
      username: myuser
      password: mypass
    queue_config:
      capacity: 10000
      max_shards: 50
      max_samples_per_send: 2000
```

### vmagent

```bash
vmagent \
  -remoteWrite.url=http://pushgw:17000/proxy/v1/write \
  -remoteWrite.basicAuth.username=myuser \
  -remoteWrite.basicAuth.password=mypass
```

### Grafana Alloy / OpenTelemetry Collector

任何兼容 Prometheus remote_write 协议的客户端都可直接对接。

---

## 与 `/prometheus/v1/write` 的对比

| 维度 | `/prometheus/v1/write` | `/proxy/v1/write` |
|------|------------------------|--------------------|
| 是否解析 body | 是（protobuf 解码、relabel、sharding） | 否（纯字节转发） |
| 内存队列 | 多分片大内存队列 | 仅 in-flight 计数 |
| Relabel / Drop / 标签改写 | 支持 | **不支持** |
| 心跳元数据更新 | 支持 | **不支持** |
| Kafka writer | 支持 | **不支持** |
| 背压机制 | 队列水位 + 丢弃 | in-flight 阈值 + 429 |
| 延迟 / CPU 开销 | 较高 | 极低 |
| 适用场景 | 需要在 pushgw 做加工 / 路由 | pushgw 只做认证 + fan-out |

简单原则：**需要 relabel、target 心跳、kafka 旁路** → 用 `/prometheus/v1/write`；**单纯透明转发到 remote_write 后端** → 用 `/proxy/v1/write`。
