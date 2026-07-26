# TDEngine

taosKeeper exposes Prometheus metrics on port `6043`.

See the [taosKeeper reference](https://docs.tdengine.com/reference/components/taoskeeper/).

The bundled dashboard uses `taos_*` metric names and therefore matches
taosKeeper's legacy `/metrics` endpoint. The real-data test used TDengine
Enterprise 3.4.2.3 and `http://taoskeeper:6043/metrics`.

## Collection configuration

Create `conf/input.prometheus/taoskeeper.toml`:

```toml
interval = 15

[[instances]]
urls = ["http://127.0.0.1:6043/metrics"]
url_label_key = "instance"
url_label_value = "{{.Host}}"
labels = { job = "taoskeeper" }
```

```bash
curl -fsS http://127.0.0.1:6043/metrics \
  | grep -E 'taos_cluster_info|taos_dn_cpu_taosd' \
  | head
```

New taosKeeper versions recommend `/metrics/v2`, which primarily emits
`taosd_*` names and is not directly compatible with the current `taos_*`
dashboard. If you migrate to v2, update the dashboard PromQL and scrape every
taosKeeper instance; each v2 instance can hold only a shard of the metrics.
