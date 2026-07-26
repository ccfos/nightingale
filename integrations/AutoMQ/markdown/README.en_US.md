## Introduction

The official AutoMQ documentation describes how metrics are exposed and how to integrate with monitoring systems. For details, refer to [AutoMQ](https://docs.automq.com/zh/docs/automq-opensource/LkwkwdQlwizjqckhj0dcc2IdnDh).

## Recommended approach

Send AutoMQ metrics to an OTel Collector Prometheus exporter, or scrape an
AutoMQ Prometheus endpoint directly. The real-data test used AutoMQ 1.5.5 and
`http://automq:8890/metrics`; use the port configured by your deployment.

Create `conf/input.prometheus/automq.toml`:

```toml
interval = 15

[[instances]]
urls = [
  "http://<automq-or-otel-collector>:8890/metrics"
]

url_label_key = "otel_collector"
url_label_value = "{{.Host}}"
labels = { source = "automq" }
```

Do not set `url_label_key` to `instance`: AutoMQ already emits an `instance`
label. Do not overwrite the original `job` label either. The dashboards use
these labels for the cluster, node, and active-controller variables.

```bash
curl -fsS http://<automq-or-otel-collector>:8890/metrics \
  | grep -E 'process_runtime_jvm_cpu_utilization_ratio|kafka_request_count_total' \
  | head
./categraf --test --inputs prometheus
```

Topic, consumer-group, and object-storage panels require real message and
object-store traffic.
