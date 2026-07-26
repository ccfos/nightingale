# Java

本目录包含两类 JVM 仪表盘，采集方式和指标名不同，不能混用：

- `JMX`、`JMX - Kubernetes`：使用 Prometheus JMX Exporter，指标包含 `jmx_exporter_build_info`、`jvm_memory_*`、`jvm_gc_collection_seconds_*`。
- `JVM by OpenTelemetry`：使用 OpenTelemetry Java Agent，经 OTel Collector 转成 Prometheus 指标。

## Prometheus JMX Exporter

为 Java 进程加载 JMX Exporter Java Agent，并让它监听例如 `9404` 端口。然后新建 Categraf 配置：

```toml
# conf/input.prometheus/java-jmx.toml
[[instances]]
urls = ["http://127.0.0.1:9404/metrics"]
url_label_key = "instance"
url_label_value = "{{.Host}}"
labels = { job = "order-service" }
```

先确认：

```bash
curl -fsS http://127.0.0.1:9404/metrics \
  | grep -E 'jmx_exporter_build_info|jvm_memory_pool_bytes_used' \
  | head
```

两张 JMX 模板通过 `job` 和 `instance` 变量筛选，必须保留这两个标签。

## OpenTelemetry Java Agent

本次使用 OpenTelemetry Java Agent 2.28.1、OTLP gRPC 和 OTel Collector 完成真实验证。Java 进程示例：

```bash
export JAVA_TOOL_OPTIONS="-javaagent:/opt/opentelemetry-javaagent.jar"
export OTEL_SERVICE_NAME="order-service"
export OTEL_EXPORTER_OTLP_ENDPOINT="http://otel-collector:4317"
export OTEL_EXPORTER_OTLP_PROTOCOL="grpc"
export OTEL_METRICS_EXPORTER="otlp"
java -jar app.jar
```

OTel Collector 最小配置：

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317

processors:
  batch: {}

exporters:
  prometheus:
    endpoint: 0.0.0.0:9464

service:
  pipelines:
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [prometheus]
```

Categraf 抓取 Collector：

```toml
# conf/input.prometheus/java-otel.toml
[[instances]]
urls = ["http://otel-collector:9464/metrics"]
url_label_key = "instance"
url_label_value = "{{.Host}}"
labels = { job = "java-otel" }
```

验证 `jvm_memory_used_bytes`、`jvm_gc_duration_seconds_*` 等 OTel JVM 指标后再导入 `JVM by OpenTelemetry`。Micrometer、JMX Exporter 和 OTel Java Agent 的指标命名不同，应选择与采集链路对应的模板。
