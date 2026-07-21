# MinIO

See [Collect MinIO Metrics Using Prometheus](https://min.io/docs/minio/linux/operations/monitoring/collect-minio-metrics-using-prometheus.html?ref=docs-redirect#minio-metrics-collect-using-prometheus)

Enable MinIO Prometheus access;

```bash
# Add the following environment variable when starting the MinIO service:
MINIO_PROMETHEUS_AUTH_TYPE=public
```

## Collection Configuration

categraf's `conf/input.prometheus/prometheus.toml`

```toml
[[instances]]
urls = [
  "http://192.168.1.188:9000/minio/v2/metrics/cluster"
]
labels = {job="minio-cluster"}
```
