# TDEngine

TDEngine 也可以暴露 Prometheus 的监控数据，具体启用方法如下：

TODO

## 采集配置

既然暴露了 Prometheus 协议的监控数据，那通过 categraf prometheus 插件直接采集即可。配置文件是 `conf/input.prometheus/prometheus.toml`。配置样例如下：

```toml
[[instances]]
urls = [
     "http://192.168.11.177:8080/xxxx"
]
```

## 仪表盘

夜莺内置了一个 TDEngine 仪表盘，由网友贡献，克隆到自己的业务组下即可使用，欢迎大家一起来提 PR 完善。