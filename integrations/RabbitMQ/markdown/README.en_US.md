# RabbitMQ

Recent versions of RabbitMQ (3.8 and above) have built-in support for exposing monitoring data in Prometheus format, so it can be collected directly with categraf's prometheus plugin. Enable RabbitMQ Prometheus access with:

```bash
rabbitmq-plugins enable rabbitmq_prometheus
```

Once enabled successfully, rabbitmq listens on port 15692 by default, and visiting `http://localhost:15692/metrics` shows monitoring data in prometheus format.

For versions below 3.8, you still need to use categraf's rabbitmq plugin to collect monitoring data.
