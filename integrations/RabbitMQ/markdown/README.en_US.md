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
