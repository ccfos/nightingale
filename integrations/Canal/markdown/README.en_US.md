## canal

Canal Server exposes Prometheus metrics. The official example uses port `11112`,
configured by `canal.metrics.pull.port` in `canal.properties`:

```properties
canal.metrics.pull.port=11112
```

Verify the endpoint:

```bash
curl -fsS http://127.0.0.1:11112/metrics | grep canal_instance | head
```

Create `conf/input.prometheus/canal.toml`:

```toml
interval = 15

[[instances]]
urls = ["http://127.0.0.1:11112/metrics"]
url_label_key = "canal_server"
url_label_value = "{{.Host}}"
labels = { job = "canal" }
```

Do not overwrite Canal's `destination` or `instance` labels because the
dashboard variables depend on them. TPS, delay, and client panels require a
running destination and real binlog traffic.

See the [Canal Prometheus QuickStart](https://github.com/alibaba/canal/wiki/Prometheus-QuickStart).
