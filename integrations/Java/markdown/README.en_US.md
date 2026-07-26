# Java

The dashboards use two incompatible metric families:

- `JMX` and `JMX - Kubernetes` require Prometheus JMX Exporter metrics such as
  `jmx_exporter_build_info` and `jvm_memory_pool_bytes_used`.
- `JVM by OpenTelemetry` requires metrics from the OpenTelemetry Java Agent.

## JMX Exporter

Load the JMX Exporter Java agent and scrape its endpoint:

```toml
[[instances]]
urls = ["http://127.0.0.1:9404/metrics"]
url_label_key = "instance"
url_label_value = "{{.Host}}"
labels = { job = "order-service" }
```

Preserve `job` and `instance`, which are dashboard variables.

## OpenTelemetry Java Agent

The real-data test used OpenTelemetry Java Agent 2.28.1, OTLP gRPC, and an OTel
Collector:

```bash
export JAVA_TOOL_OPTIONS="-javaagent:/opt/opentelemetry-javaagent.jar"
export OTEL_SERVICE_NAME="order-service"
export OTEL_EXPORTER_OTLP_ENDPOINT="http://otel-collector:4317"
export OTEL_EXPORTER_OTLP_PROTOCOL="grpc"
export OTEL_METRICS_EXPORTER="otlp"
java -jar app.jar
```

Collector:

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

Categraf:

```toml
[[instances]]
urls = ["http://otel-collector:9464/metrics"]
url_label_key = "instance"
url_label_value = "{{.Host}}"
labels = { job = "java-otel" }
```

Verify `jvm_memory_used_bytes` and `jvm_gc_duration_seconds_*` before importing
the OpenTelemetry dashboard.
