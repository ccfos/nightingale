# Gitlab

Gitlab exposes monitoring data in Prometheus format by default; see [Monitoring GitLab with Prometheus](https://docs.gitlab.com/ee/administration/monitoring/prometheus/). Therefore, you can simply use categraf's prometheus plugin to collect it.

## Collection Configuration

Configuration file: categraf's `conf/input.prometheus/prometheus.toml`

```toml
[[instances]]
urls = [
  "http://192.168.11.77:9236/metrics"
]
labels = {service="gitlab", job="gitaly"}

[[instances]]
urls = [
  "http://192.168.11.77:9168/sidekiq"
]
labels = {service="gitlab", job="gitlab-exporter-sidekiq"}

[[instances]]
urls = [
  "http://192.168.11.77:9168/database"
]
labels = {service="gitlab",job="gitlab-exporter-database"}

[[instances]]
urls = [
  "http://192.168.11.77:8082/metrics"
]
labels = {service="gitlab", job="gitlab-sidekiq"}

[[instances]]
urls = [
  "http://192.168.11.77:8082/metrics"
]
labels = {service="gitlab", job="gitlab-sidekiq"}

[[instances]]
urls = [
  "http://192.168.11.77:9229/metrics"
]
labels = {service="gitlab",job="gitlab-workhorse"}

[[instances]]
urls = [
  "http://192.168.11.77:9100/metrics"
]
labels = {service="gitlab", job="node"}

[[instances]]
urls = [
  "http://192.168.11.77:9187/metrics"
]
labels = {service="gitlab", job="postgres"}

[[instances]]
urls = [
  "http://192.168.11.77:9121/metrics"
]
labels = {service="gitlab", job="redis"}

[[instances]]
urls = [
  "http://192.168.11.77:9999/metrics"
]
labels = {service="gitlab", job="nginx"}
```

