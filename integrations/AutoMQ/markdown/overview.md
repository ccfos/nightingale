## 前言

AuthMQ 官方文档提供了指标吐出方式以及和监控系统的整合方式，具体可以参考[AutoMQ](https://docs.automq.com/zh/docs/automq-opensource/LkwkwdQlwizjqckhj0dcc2IdnDh)。

## 推荐方式

建议采用 AutoMQ 文档中的方案二：使用 Prometheus OTLP Receiver 的方式，把所有的指标都收集到 OTel Collector 中，然后使用 Prometheus 或者 Categraf 直接去拉取数据即可。假如使用 Categraf，就是使用 prometheus 插件去拉取数据，比如我们为 prometheus 插件提供一个单独的 automq.toml 的配置文件：`conf/input.prometheus/automq.toml` ，内容如下：

```toml
[[instances]]
urls = [
     "http://<otel-collector-ip>:<otel-collector-port>/metrics"
]

url_label_key = "otel_collector"
url_label_value = "{{.Host}}"
```

注意，url_label_key 一般都是指定为 instance，但是这里故意指定为其他字符串，是因为 AutoMQ 原始的指标中包含了 instance 标签，为了避免冲突，所以指定为其他字符串。

