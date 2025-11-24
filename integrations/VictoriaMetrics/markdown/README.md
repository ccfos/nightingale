# VictoriaMetrics

VictoriaMetrics 既可以单机部署，也可以集群方式部署。不管哪种部署方式，VictoriaMetrics 的进程都会暴露 `/metrics` 接口，通过这个接口暴露 Prometheus 协议的监控数据。

## 采集配置

categraf 的 `conf/input.prometheus/prometheus.toml`。因为 VictoriaMetrics 是暴露的 Prometheus 协议的监控数据，所以使用 categraf 的 prometheus 插件即可采集。

```toml
# vmstorage
[[instances]]
urls = [
     "http://127.0.0.1:8482/metrics"
]
labels = {service="vmstorage"}

# vmselect
[[instances]]
urls = [
     "http://127.0.0.1:8481/metrics"
]

labels = {service="vmselect"}

# vminsert
[[instances]]
urls = [
     "http://127.0.0.1:8480/metrics"
]
labels = {service="vminsert"}
```

