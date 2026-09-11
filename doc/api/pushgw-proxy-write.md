# Pushgw Proxy Remote Write API

Nightingale pushgw provides a **pure forwarding** remote_write endpoint: the client pushes Prometheus remote_write data to pushgw, and pushgw forwards the raw bytes to each backend in the `Writers` list from the configuration file (Prometheus / VictoriaMetrics / Mimir / any other storage that speaks the remote_write protocol) without parsing the body or going through a queue.

Compared with `/prometheus/v1/write`, which goes through the full in-memory queue + relabel + sharding pipeline, `/proxy/v1/write` behaves much more like "an L7 reverse proxy with authentication and concurrency protection". It suits cases where:

- pushgw is only meant to be an ingest gateway and write aggregation is left to the backend cluster;
- you need multi-datacenter / multi-replica fan-out, replicating the same data to several backends;
- you want the client's headers (`Content-Encoding`, `X-Prometheus-Remote-Write-Version`, and so on) preserved verbatim instead of being repackaged by pushgw.

---

## Endpoint

```
POST /proxy/v1/write
```

- Request body: standard Prometheus remote_write, i.e. protobuf with snappy compression.
- pushgw never parses the body and forwards it verbatim; the query string is appended verbatim to each writer's URL as well.
- Whether authentication is required is decided by the pushgw configuration (see `BasicAuth` below).

### Authentication

Whether authentication is enabled is controlled by `HTTP.APIForAgent.BasicAuth` / `HTTP.APIForService.BasicAuth` in `pushgw.yaml`:

- If either map is non-empty → HTTP Basic Auth is enabled, and the request must carry `Authorization: Basic <base64(user:pass)>`;
- If both are empty → no authentication is required.

Example:

```bash
curl -u myuser:mypass \
  -H 'Content-Type: application/x-protobuf' \
  -H 'Content-Encoding: snappy' \
  -H 'X-Prometheus-Remote-Write-Version: 0.1.0' \
  --data-binary @payload.snappy \
  "http://pushgw:17000/proxy/v1/write"
```

### Header pass-through rules

The following request headers are passed through by pushgw to the backend writer; when absent, the default is used:

| Header | Default | Description |
|--------|--------|------|
| `Content-Type` | `application/x-protobuf` | remote_write standard |
| `Content-Encoding` | `snappy` | remote_write standard |
| `User-Agent` | `n9e` | If the client sends one, a `-n9e` suffix is appended (for example `prometheus/2.45.0-n9e`) |
| `X-Prometheus-Remote-Write-Version` | `0.1.0` | remote_write protocol version |

No other request headers are passed through. A writer's own `BasicAuthUser` / `BasicAuthPass` / `Headers` are set separately when forwarding (configured in the configuration file, see below).

### Query string pass-through

The query string of the request URL is appended verbatim to each writer's URL. For example:

- Writer configuration: `http://vminsert:8480/insert/0/prometheus/api/v1/write`
- Client request: `POST /proxy/v1/write?extra_label=cluster%3Dcn-bj`
- Actually forwarded: `POST http://vminsert:8480/insert/0/prometheus/api/v1/write?extra_label=cluster%3Dcn-bj`

If the writer URL already contains a `?`, an `&` is used instead.

---

## Response

### Success

```
HTTP/1.1 200 OK
```

Note: **as soon as pushgw has received the data and is ready to forward it, it returns 200**. Whether a backend writer succeeded (including 4xx/5xx, timeouts, and connection failures) is reflected only in logs and metrics and is **never propagated back to the client response**. This follows inevitably from the fan-out / multi-writer design — one slow or dead writer must not affect the whole request.

### Failure

| Status | Response | Trigger |
|--------|------|----------|
| 400 | `{"error": "..."}` | Reading the body failed (connection reset, client closed, etc.) |
| 413 | `proxy remote write body too large: > <N> bytes` | A single request body exceeds `ProxyMaxBodyBytes` |
| 429 | `proxy remote write inflight over limit: <N>` | The number of concurrent in-flight requests exceeds `ProxyInflightMax` |

429 is a **backpressure** signal. Combined with the remote_write client's native WAL and backoff-retry mechanism, the client retries automatically and no handling is needed at a higher layer.

---

## Backpressure and rate limiting

`/proxy/v1/write` bounds memory usage with two global parameters:

| Setting | Default | Description |
|--------|--------|------|
| `Pushgw.ProxyInflightMax` | `1000` | Concurrency limit for a single pushgw process. Exceeding it returns 429 immediately, and the request is **not** counted toward writer forwarding |
| `Pushgw.ProxyMaxBodyBytes` | `32 * 1024 * 1024` (32 MiB) | Maximum body size of a single request; exceeding it returns 413 |

Example `pushgw.yaml`:

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

> The memory ceiling is roughly `ProxyInflightMax × ProxyMaxBodyBytes`, i.e. about 1000 × 32 MiB ≈ 32 GiB, but the real peak is far lower (most remote_write batches are 64–256 KiB).

### Writers configuration

| Field | Type | Description |
|------|------|------|
| `Url` | string | Backend remote_write address (required) |
| `BasicAuthUser` / `BasicAuthPass` | string | Basic-auth credentials for the backend |
| `Timeout` | int (ms) | Timeout for the whole request |
| `DialTimeout` | int (ms) | TCP connection timeout |
| `MaxConnsPerHost` / `MaxIdleConns` / `MaxIdleConnsPerHost` | int | HTTP connection-pool parameters |
| `IdleConnTimeout` / `KeepAlive` / `TLSHandshakeTimeout` / `ExpectContinueTimeout` | int (ms) | Various HTTP transport-layer timeouts |
| `Headers` | []string | Custom request headers, written in pairs as `[key1, val1, key2, val2, ...]`. If a key is `Host`, `req.Host` is set as well |

> Note: `/proxy/v1/write` does not use `WriteRelabels` — it cannot relabel what it does not parse. Relabeling only takes effect on the `/prometheus/v1/write` path.

---

## Metrics

pushgw exposes the following metrics on `/metrics` (namespace `n9e_pushgw`):

| Metric | Type | Labels | Description |
|------|------|------|------|
| `n9e_pushgw_proxy_remote_write_total` | Counter | - | Total number of `/proxy/v1/write` requests received |
| `n9e_pushgw_proxy_remote_write_inflight` | Gauge | - | Current number of in-flight requests (the key metric for observing backpressure) |
| `n9e_pushgw_proxy_remote_write_over_limit_total` | Counter | - | Number of requests rejected with 429 because the in-flight limit was exceeded |
| `n9e_pushgw_proxy_remote_write_body_too_large_total` | Counter | - | Number of requests rejected with 413 because the body limit was exceeded |
| `n9e_pushgw_proxy_forward_total` | Counter | `url` | Number of forwards issued to each writer |
| `n9e_pushgw_proxy_forward_error_total` | Counter | `url`, `reason` | Number of failed forwards. `reason` is one of `build_request` / `do_request` / `status_4xx_5xx` |
| `n9e_pushgw_proxy_forward_duration_seconds` | Histogram | `url` | Distribution of per-forward latency |

Recommended alerts:

- `n9e_pushgw_proxy_remote_write_inflight` staying close to `ProxyInflightMax` → scale out or raise the threshold;
- `rate(n9e_pushgw_proxy_remote_write_over_limit_total[5m]) > 0` firing continuously → backend writes are slow and clients may be losing data;
- `rate(n9e_pushgw_proxy_forward_error_total[5m]) > 0` → slice by `url` / `reason` to troubleshoot.

---

## Client configuration examples

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

Any client that speaks the Prometheus remote_write protocol can connect directly.

---

## Comparison with `/prometheus/v1/write`

| Aspect | `/prometheus/v1/write` | `/proxy/v1/write` |
|------|------------------------|--------------------|
| Parses the body | Yes (protobuf decoding, relabel, sharding) | No (raw byte forwarding) |
| In-memory queue | Large multi-shard in-memory queue | In-flight counter only |
| Relabel / drop / label rewriting | Supported | **Not supported** |
| Heartbeat metadata updates | Supported | **Not supported** |
| Kafka writer | Supported | **Not supported** |
| Backpressure mechanism | Queue watermark + dropping | In-flight threshold + 429 |
| Latency / CPU overhead | Higher | Very low |
| Best for | Processing or routing inside pushgw | pushgw doing only authentication + fan-out |

A simple rule of thumb: **if you need relabeling, target heartbeats, or the Kafka side channel** → use `/prometheus/v1/write`; **if you just want transparent forwarding to remote_write backends** → use `/proxy/v1/write`.
