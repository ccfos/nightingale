## 前言

AutoMQ 官方文档提供了指标输出方式以及和监控系统的整合方式，具体可以参考 [AutoMQ](https://docs.automq.com/zh/docs/automq-opensource/LkwkwdQlwizjqckhj0dcc2IdnDh)。

## 推荐方式

建议把 AutoMQ 指标汇聚到 OTel Collector 的 Prometheus exporter，或者直接抓取 AutoMQ 已暴露的 Prometheus 端点，再由 Categraf 的 Prometheus input 拉取。

本次实测使用 AutoMQ 1.5.5，指标端点为 `http://automq:8890/metrics`。不同部署方式的端口可能不同，请以 AutoMQ 或 OTel Collector 的实际配置为准。

为 Prometheus input 新建 `conf/input.prometheus/automq.toml`：

```toml
interval = 15

[[instances]]
urls = [
  "http://<automq-or-otel-collector>:8890/metrics"
]

url_label_key = "otel_collector"
url_label_value = "{{.Host}}"
labels = { source = "automq" }
```

`url_label_key` 不要设置为 `instance`。AutoMQ 原始指标已经包含 `instance`，覆盖它会导致仪表盘的 `cluster_id`、`node_id` 或 active controller 变量无法匹配。模板还依赖 AutoMQ 输出的 `job` 标签作为集群标识，不要在采集侧统一覆盖 `job`。

## 验证

```bash
curl -fsS http://<automq-or-otel-collector>:8890/metrics \
  | grep -E 'process_runtime_jvm_cpu_utilization_ratio|kafka_request_count_total' \
  | head

./categraf --test --inputs prometheus
```

只有 AutoMQ 实际产生 Topic、Consumer Group 和对象存储读写后，相应面板才会出现数据。
