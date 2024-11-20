# Doris

Doris 的进程都会暴露 `/metrics` 接口，通过这个接口暴露 Prometheus 协议的监控数据。

## 采集配置

categraf 的 `conf/input.prometheus/prometheus.toml`。因为 Doris 是暴露的 Prometheus 协议的监控数据，所以使用 categraf 的 prometheus 插件即可采集。

```toml
# doris_fe
[[instances]]
urls = [
     "http://127.0.0.1:8030/metrics"
]

url_label_key = "instance"
url_label_value = "{{.Host}}"

labels = { group = "fe",job = "doris_cluster01"}

# doris_be
[[instances]]
urls = [
     "http://127.0.0.1:8040/metrics"
]
url_label_key = "instance"
url_label_value = "{{.Host}}"
labels = { group = "be",job = "doris_cluster01"}
```

## 告警规则

夜莺内置了 Doris 的告警规则，克隆到自己的业务组下即可使用。

## 仪表盘

夜莺内置了 Doris 的仪表盘，克隆到自己的业务组下即可使用。


