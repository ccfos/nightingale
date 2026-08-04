# VictoriaMetrics

VictoriaMetrics 既可以单机部署，也可以集群方式部署。不管哪种部署方式，VictoriaMetrics 的进程都会暴露 `/metrics` 接口，通过这个接口暴露 Prometheus 协议的监控数据。

## 单机版

单机版默认端口为 `8428`：

```toml
[[instances]]
urls = ["http://127.0.0.1:8428/metrics"]
url_label_key = "instance"
url_label_value = "{{.Host}}"
labels = { job = "victoriametrics", service = "vmsingle" }
```

## 集群版

集群版需要分别采集 vmstorage、vmselect 和 vminsert，不能只抓一个组件：

```toml
[[instances]]
urls = ["http://127.0.0.1:8482/metrics"]
labels = { job = "victoriametrics", service = "vmstorage" }

[[instances]]
urls = ["http://127.0.0.1:8481/metrics"]
labels = { job = "victoriametrics", service = "vmselect" }

[[instances]]
urls = ["http://127.0.0.1:8480/metrics"]
labels = { job = "victoriametrics", service = "vminsert" }
```

本次实测以 single 部署为主，因此 `VictoriaMetrics - Single` 可完整使用，而 `VictoriaMetrics - Cluster` 中多副本、租户和集群组件专属面板可能为空。不要通过给 single 指标伪造 `vmstorage`/`vmselect`/`vminsert` 标签来冒充集群验证。
