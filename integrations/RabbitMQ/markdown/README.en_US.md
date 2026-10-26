# RabbitMQ

RabbitMQ 3.8 and later include a Prometheus plugin. Enable it:

```bash
rabbitmq-plugins enable rabbitmq_prometheus
```

It listens on port `15692` by default:

```bash
curl -fsS http://127.0.0.1:15692/metrics | grep rabbitmq_build_info
```

Create `conf/input.prometheus/rabbitmq.toml`:

```toml
[[instances]]
urls = ["http://127.0.0.1:15692/metrics"]
url_label_key = "instance"
url_label_value = "{{.Host}}"
labels = { job = "rabbitmq" }
```

The 3.8/3.8+ dashboards were validated against built-in Prometheus metrics.
Create exchanges and queues and publish/consume messages before checking rate
and backlog panels.

For versions older than 3.8, enable `rabbitmq_management` and use Categraf's
RabbitMQ input against port `15672`. Its metric names differ, so use the
matching legacy dashboard.

## Collect templates

Two templates ship under `collect`:

| Template | Input | When to use |
| --- | --- | --- |
| rabbitmq_exporter | rabbitmq_exporter | Categraf embeds the kbudde/rabbitmq_exporter collection logic and talks to the Management API (port 15672), keeping the exporter's metric names such as `rabbitmq_up` and `rabbitmq_queue_messages_ready`. `rabbit_url` is required, the input stays off until it is set |
| rabbitmq | rabbitmq | Forked from telegraf/rabbitmq, also on the Management API, but emits `rabbitmq_overview_*` / `rabbitmq_node_*` names |

For a fresh 3.8+ environment prefer the prometheus input against port 15692 as
described above — the 3.8+ dashboards here are built on those native metrics.
The three paths use different metric names, so pick the collection method and
the dashboard as a pair.
