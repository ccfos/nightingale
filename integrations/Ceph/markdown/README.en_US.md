# ceph plugin

Enable ceph prometheus support

```bash
ceph mgr module enable prometheus
```

## Collection Configuration

Since ceph can expose metrics data in prometheus format, you can simply scrape it with the prometheus plugin.

categraf configuration file: `conf/input.prometheus/prometheus.toml`

```yaml
[[instances]]
urls = [
  "http://192.168.11.181:9283/metrics"
]
labels = {service="ceph",cluster="ceph-cluster-001"}
```


## Dashboard

Nightingale's built-in dashboards already include a dashboard for ceph — just import it and start using it.

![20230801152445](https://download.flashcat.cloud/ulric/20230801152445.png)

## Alert Rules

Nightingale's built-in alert rules already include alert rules for ceph — just import them and start using them.

![20230801152431](https://download.flashcat.cloud/ulric/20230801152431.png)
