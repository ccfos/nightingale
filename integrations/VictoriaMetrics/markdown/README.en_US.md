# VictoriaMetrics

VictoriaMetrics can be deployed either as a single node or as a cluster. In either deployment mode, VictoriaMetrics processes expose a `/metrics` endpoint that serves monitoring data in the Prometheus format.

## Single-node

```toml
[[instances]]
urls = ["http://127.0.0.1:8428/metrics"]
url_label_key = "instance"
url_label_value = "{{.Host}}"
labels = { job = "victoriametrics", service = "vmsingle" }
```

## Cluster

Scrape vmstorage, vmselect, and vminsert separately:

```toml
[[instances]]
urls = ["http://127.0.0.1:8482/metrics"]
labels = { job = "victoriametrics", service = "vmstorage" }

[[instances]]
urls = ["http://127.0.0.1:8481/metrics"]
labels = { job = "victoriametrics", service = "vmselect" }

[[instances]]
urls = ["http://127.0.0.1:8480/metrics"]
labels = { job = "victoriametrics", service = "vminsert" }
```

The real-data test primarily used a single-node deployment. Cluster-only
replica, tenant, and component panels are expected to be empty in that setup;
do not fake cluster service labels on single-node metrics.
