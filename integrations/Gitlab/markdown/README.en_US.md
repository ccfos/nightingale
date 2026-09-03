# Gitlab

The five dashboards require metrics from the host, Rails/Sidekiq, Gitaly,
Workhorse, PostgreSQL, Redis, and NGINX. Scraping a single GitLab endpoint is
not sufficient. See [Monitoring GitLab with Prometheus](https://docs.gitlab.com/administration/monitoring/prometheus/).

For an external Categraf, configure the required exporters in
`/etc/gitlab/gitlab.rb` to listen on reachable addresses, restrict access with
the firewall, add Categraf to `gitlab_rails['monitoring_whitelist']`, and run
`gitlab-ctl reconfigure`.

## Collection Configuration

Create `conf/input.prometheus/gitlab.toml`. The following is a single-node
example; scrape every relevant node in a distributed deployment.

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
  "http://gitlab.example.com/-/metrics"
]
labels = {service="gitlab", job="gitlab-rails"}

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
  "http://192.168.11.77:8060/metrics"
]
labels = {service="gitlab", job="nginx"}
```

Do not scrape the same Sidekiq endpoint twice. The exporter endpoints on 9168
(`/database` and `/sidekiq`) and Sidekiq's own 8082 `/metrics` endpoint are
different. Generate real web, API, and Git traffic before validating rate
panels. Most exporters do not authenticate clients, so never expose them
directly to the public Internet.
