# Doris

Doris processes all expose a `/metrics` endpoint, through which monitoring data in Prometheus format is exposed.

## Collection configuration

Use categraf's `conf/input.prometheus/prometheus.toml`. Since Doris exposes monitoring data in Prometheus format, it can be collected with categraf's prometheus plugin.

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

