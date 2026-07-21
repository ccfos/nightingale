# N9E

Nightingale V5 consists of two components, n9e-webapi and n9e-server, both of which expose Prometheus-format monitoring data through the `/metrics` endpoint. Nightingale V6 has only one component by default, n9e, which also exposes Prometheus-format monitoring data through the `/metrics` endpoint. If you use the edge deployment architecture, n9e-edge is involved as well, and it also exposes Prometheus-format monitoring data through the `/metrics` endpoint.

Therefore, Nightingale's monitoring data can be collected simply with categraf's prometheus plugin.

## Collector configuration

categraf's `conf/input.prometheus/prometheus.toml`:

```toml
[[instances]]
urls = [
  "http://IP:17000/metrics"
]
labels = {job="n9e"}
```
