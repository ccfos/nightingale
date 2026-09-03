# MinIO

参考 [使用 Prometheus 采集 MinIO 指标](https://min.io/docs/minio/linux/operations/monitoring/collect-minio-metrics-using-prometheus.html?ref=docs-redirect#minio-metrics-collect-using-prometheus)。

测试环境使用公开指标端点：

```bash
MINIO_PROMETHEUS_AUTH_TYPE=public
```

生产环境也可以保留鉴权，并在 Categraf 中配置相应 Bearer Token。不要为了监控直接把指标端点暴露到公网。

## 采集配置

两张 MinIO 模板同时使用 cluster、bucket 和 node 三类指标，只抓 `/cluster` 会导致对象、Bucket、进程和磁盘面板缺数据。新建 `conf/input.prometheus/minio.toml`：

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

三组端点的 `job` 必须保持一致，仪表盘通过 `job` 变量统一筛选。至少创建一个 bucket 并执行对象上传、下载和删除，再验证请求、流量、容量及对象面板。

集群和 bucket 端点通常抓取负载均衡地址；node 端点应抓取每个 MinIO 节点，并为每个节点设置不同的 `instance`。上面的 `ignore_metrics` 与本次实测一致，用于避免 node 端点重复上报 cluster/S3/Go 指标。

```bash
for path in cluster bucket node; do
  curl -fsS "http://127.0.0.1:9000/minio/v2/metrics/${path}" | head
done
```

MinIO 已推荐新部署使用 metrics v3，但当前两张模板查询的是 v2 `minio_*` 指标。改用 `/minio/metrics/v3` 前需要同步升级仪表盘 PromQL。
