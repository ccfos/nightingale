# MinIO

参考 [使用 Prometheus 采集 MinIO 指标](https://min.io/docs/minio/linux/operations/monitoring/collect-minio-metrics-using-prometheus.html?ref=docs-redirect#minio-metrics-collect-using-prometheus)

开启 MinIO Prometheus 访问；

```bash
# 启动 MinIO 服务的时候加入下面的变量：
MINIO_PROMETHEUS_AUTH_TYPE=public
```

## 采集配置

categraf 的 `conf/input.prometheus/prometheus.toml`

```toml
[[instances]]
urls = [
  "http://192.168.1.188:9000/minio/v2/metrics/cluster"
]
labels = {job="minio-cluster"}
```

## Dashboard

夜莺内置了 MinIO 的仪表盘，克隆到自己的业务组下即可使用。

![20230801170735](https://download.flashcat.cloud/ulric/20230801170735.png)

## Alerts

夜莺内置了 MinIO 的告警规则，克隆到自己的业务组下即可使用。

![20230801170725](https://download.flashcat.cloud/ulric/20230801170725.png)