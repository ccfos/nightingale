# N9E

夜莺V5版本分两个组件，n9e-webapi 和 n9e-server，都通过 `/metrics` 接口暴露了 Prometheus 协议的监控数据。夜莺V6版本默认只有一个组件，就是 n9e，也通过 `/metrics` 接口暴露了 Prometheus 协议的监控数据。如果使用边缘机房部署方案，会用到 n9e-edge，n9e-edge 也通过 `/metrics` 接口暴露了 Prometheus 协议的监控数据。

所以，通过 categraf 的 prometheus 插件即可采集夜莺的监控数据。

## 采集配置

categraf 的 `conf/input.prometheus/prometheus.toml`

```toml
[[instances]]
urls = [
  "http://IP:17000/metrics"
]
labels = {job="n9e"}
```

## Dashboard

夜莺内置了两个 N9E 仪表盘，n9e_server 是给 V5 版本用的，n9e_v6 是给 V6 版本用的。

