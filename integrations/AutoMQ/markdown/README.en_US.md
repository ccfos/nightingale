## Introduction

The official AutoMQ documentation describes how metrics are exposed and how to integrate with monitoring systems. For details, refer to [AutoMQ](https://docs.automq.com/zh/docs/automq-opensource/LkwkwdQlwizjqckhj0dcc2IdnDh).

## Recommended approach

We recommend option 2 from the AutoMQ documentation: use the Prometheus OTLP Receiver approach to collect all metrics into the OTel Collector, and then have Prometheus or Categraf pull the data directly. If you use Categraf, that means pulling the data with the prometheus plugin. For example, we can provide a dedicated automq.toml configuration file for the prometheus plugin at `conf/input.prometheus/automq.toml`, with the following content:

```toml
[[instances]]
urls = [
     "http://<otel-collector-ip>:<otel-collector-port>/metrics"
]

url_label_key = "otel_collector"
url_label_value = "{{.Host}}"
```

Note that url_label_key is usually set to instance, but here it is deliberately set to a different string because the original AutoMQ metrics already contain an instance label. To avoid conflicts, a different string is used.
