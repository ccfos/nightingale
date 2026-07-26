# Gitlab

GitLab Linux Package 内置多个 exporter。完整使用本目录的 5 张模板，需要同时采集主机、Rails/Sidekiq、Gitaly、Workhorse、PostgreSQL、Redis 和 NGINX 指标，而不是只抓一个 `/metrics`。

参考：[Monitoring GitLab with Prometheus](https://docs.gitlab.com/administration/monitoring/prometheus/)。

## GitLab 侧配置

外部 Categraf 抓取时，应在 `/etc/gitlab/gitlab.rb` 中让需要的端点监听可达地址，并用防火墙只允许采集节点访问：

```ruby
prometheus['enable'] = false
node_exporter['listen_address'] = '0.0.0.0:9100'
gitlab_workhorse['prometheus_listen_addr'] = '0.0.0.0:9229'
gitlab_exporter['listen_address'] = '0.0.0.0'
gitlab_exporter['listen_port'] = '9168'
redis_exporter['listen_address'] = '0.0.0.0:9121'
postgres_exporter['listen_address'] = '0.0.0.0:9187'
gitaly['configuration'] = {
  prometheus_listen_addr: '0.0.0.0:9236'
}
```

Rails 的 `/-/metrics` 还需要把 Categraf 地址加入 `gitlab_rails['monitoring_whitelist']`。修改后执行 `gitlab-ctl reconfigure`。

## 采集配置

为 Categraf 新建 `conf/input.prometheus/gitlab.toml`。以下端点按单机 GitLab 示例给出；多节点部署应抓取每个对应节点：

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
  "http://gitlab.example.com/-/metrics"
]
labels = {service="gitlab", job="gitlab-rails"}

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
  "http://192.168.11.77:8060/metrics"
]
labels = {service="gitlab", job="nginx"}
```

不要重复抓取同一 Sidekiq 端点，否则会产生重复时序。GitLab exporter（9168）的 `/database` 和 `/sidekiq` 与 Sidekiq 自身的 8082 `/metrics` 不是同一个端点，都应按模板需要保留。

验证时应实际访问 GitLab 页面、执行 Git 操作或 API 请求，并确认：

```promql
up{job=~"gitaly|gitlab-workhorse|gitlab-rails|gitlab-sidekiq|postgres|redis|nginx|node"} == 1
```

大多数 exporter 没有鉴权，不要直接暴露到公网。
