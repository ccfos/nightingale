# Logstash

Logstash 默认在 HTTP API 上暴露节点、JVM 和 pipeline 统计信息，Categraf 的 Logstash input 通过这个 API 采集。默认 API 地址通常是 `http://127.0.0.1:9600`。

先确认接口可访问：

```bash
curl -fsS http://127.0.0.1:9600/_node/stats | head
```

## Categraf 配置

配置文件为 `conf/input.logstash/logstash.toml`：

```toml
[[instances]]
url = "http://127.0.0.1:9600"
collect = ["pipelines", "process", "jvm"]
timeout = "5s"
labels = { instance = "logstash-01" }
```

如果 Logstash API 启用了 Basic Auth 或 TLS，应同时配置 `username`、`password` 和 TLS 证书参数。模板中的事件吞吐、队列和 pipeline 面板只有在 Logstash 实际处理事件后才会出现非零数据。

参考配置：[Categraf Logstash input](https://github.com/flashcatcloud/categraf/blob/main/conf/input.logstash/logstash.toml)。
