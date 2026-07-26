## canal

Canal Server 提供 Prometheus 格式指标，可以直接通过 Categraf 的 Prometheus input 采集。官方默认示例端口是 `11112`，对应 `canal.properties` 中的 `canal.metrics.pull.port`。

## Canal 配置

确认 `canal.properties` 中启用了指标端口：

```properties
canal.metrics.pull.port=11112
```

启动 Canal 后先确认端点能够返回指标：

```bash
curl -fsS http://127.0.0.1:11112/metrics | grep canal_instance | head
```

## Categraf 配置

新建 `conf/input.prometheus/canal.toml`：

```toml
interval = 15

[[instances]]
urls = ["http://127.0.0.1:11112/metrics"]
url_label_key = "canal_server"
url_label_value = "{{.Host}}"
labels = { job = "canal" }
```

不要覆盖 Canal 原始指标中的 `destination` 和 `instance` 标签，仪表盘变量依赖这些标签。只启动 Canal Server 但没有创建 destination 或没有 binlog 读写时，TPS、延迟和 Client 面板为空是正常现象。

参考：[Canal Prometheus QuickStart](https://github.com/alibaba/canal/wiki/Prometheus-QuickStart)。
