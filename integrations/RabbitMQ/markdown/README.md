# RabbitMQ

高版本（3.8以上版本）的 RabbitMQ，已经内置支持了暴露 Prometheus 协议的监控数据。所以，直接使用 categraf 的 prometheus 插件即可采集。开启 RabbitMQ Prometheus 访问：

```bash
rabbitmq-plugins enable rabbitmq_prometheus
```

启用成功的话，rabbitmq 默认会在 15692 端口起监听，访问 `http://localhost:15692/metrics` 即可看到符合 prometheus 协议的监控数据。

如果低于 3.8 的版本，还是需要使用 categraf 的 rabbitmq 插件来采集监控数据。
