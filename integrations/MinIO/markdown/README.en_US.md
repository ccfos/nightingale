# MinIO

See [Collect MinIO Metrics Using Prometheus](https://min.io/docs/minio/linux/operations/monitoring/collect-minio-metrics-using-prometheus.html?ref=docs-redirect#minio-metrics-collect-using-prometheus)

The test environment used public metrics:

```bash
MINIO_PROMETHEUS_AUTH_TYPE=public
```

Production deployments can keep authentication and configure the corresponding
Bearer token in Categraf. Do not expose metrics publicly.

## Collection Configuration

Both dashboards use cluster, bucket, and node metrics. Scraping only `/cluster`
leaves object, bucket, process, and disk panels empty.

```toml
[[instances]]
urls = ["http://127.0.0.1:9000/minio/v2/metrics/cluster"]
labels = { job = "minio", instance = "minio-01:9000" }

[[instances]]
urls = ["http://127.0.0.1:9000/minio/v2/metrics/bucket"]
labels = { job = "minio", instance = "minio-01:9000" }

[[instances]]
urls = ["http://127.0.0.1:9000/minio/v2/metrics/node"]
ignore_metrics = [
  "go_*",
  "minio_audit_*",
  "minio_cluster_*",
  "minio_node_replication_*",
  "minio_node_storage_class_*",
  "minio_notify_*",
  "minio_s3_*",
  "minio_software_*"
]
labels = { job = "minio", instance = "minio-01:9000" }
```

Keep the same `job` across all three endpoints. Create a bucket and perform
real object uploads, downloads, and deletes before validating the dashboards.
Scrape cluster/bucket from the load balancer and scrape the node endpoint on
every MinIO node with a unique `instance`. The ignore list prevents duplicate
cluster, S3, and Go series from the node endpoint.

MinIO recommends metrics v3 for new deployments, but these dashboards query v2
`minio_*` names. Update the dashboard PromQL before switching to
`/minio/metrics/v3`.
