# RabbitMQ

RabbitMQ 3.8 及以上内置 Prometheus 插件，推荐直接使用 Categraf Prometheus input 采集。开启插件：

```bash
rabbitmq-plugins enable rabbitmq_prometheus
```

默认监听 `15692`：

```bash
curl -fsS http://127.0.0.1:15692/metrics | grep rabbitmq_build_info
```

新建 `conf/input.prometheus/rabbitmq.toml`：

```toml
interval = 15

[[instances]]
urls = ["http://127.0.0.1:15692/metrics"]
url_label_key = "instance"
url_label_value = "{{.Host}}"
labels = { job = "rabbitmq" }
```

本目录的 3.8/3.8+ 模板按内置 Prometheus 指标验证。应创建真实 exchange、queue，并执行 publish/consume，才能看到吞吐、积压和确认相关面板。

RabbitMQ 低于 3.8 时，可启用 `rabbitmq_management` 并使用 Categraf `input.rabbitmq` 访问默认管理端口 `15672`；这条链路的指标名与 3.8+ Prometheus 模板并不完全相同，应选择对应的旧版模板。

## 采集模板

本目录的 `collect` 下提供两个采集模板：

| 模板 | 插件 | 适用场景 |
| --- | --- | --- |
| rabbitmq_exporter | rabbitmq_exporter | categraf 内部集成 kbudde/rabbitmq_exporter 的采集逻辑，走 Management API（默认 15672 端口），指标名与独立部署的 exporter 兼容，如 `rabbitmq_up`、`rabbitmq_queue_messages_ready`。`rabbit_url` 必填，不填则插件不启用 |
| rabbitmq | rabbitmq | fork 自 telegraf/rabbitmq，同样走 Management API，指标名是 `rabbitmq_overview_*`、`rabbitmq_node_*` 这一套 |

3.8 及以上的新环境推荐走上面说的 prometheus 插件抓 15692 端口，本目录的 3.8+ 大盘也是按那套原生指标做的。三条链路指标名互不相同，采集方式和大盘要成对选择。
