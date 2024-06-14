# Gitlab

Gitlab 默认提供 Prometheus 协议的监控数据，参考：[Monitoring GitLab with Prometheus](https://docs.gitlab.com/ee/administration/monitoring/prometheus/)。所以，使用 categraf 的 prometheus 插件即可采集。

## 采集配置

配置文件：categraf 的 `conf/input.prometheus/prometheus.toml`

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

## 仪表盘和告警规则

夜莺内置提供了 gitlab 各个组件相关的仪表盘和告警规则，导入自己的业务组即可使用。

