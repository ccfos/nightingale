# VictoriaMetrics

VictoriaMetrics can be deployed either as a single node or as a cluster. In either deployment mode, VictoriaMetrics processes expose a `/metrics` endpoint that serves monitoring data in the Prometheus format.

## Collection Configuration

categraf's `conf/input.prometheus/prometheus.toml`. Since VictoriaMetrics exposes monitoring data in the Prometheus format, you can simply use categraf's prometheus plugin to collect it.

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
