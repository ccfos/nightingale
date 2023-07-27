# redis

redis 的监控原理，就是连上 redis，执行 info 命令，解析结果，整理成监控数据上报。

## Configuration

redis 插件的配置在 `conf/input.redis/redis.toml` 最简单的配置如下：

```toml
[[instances]]
address = "127.0.0.1:6379"
username = ""
password = ""
labels = { instance="n9e-10.23.25.2:6379" }
```

如果要监控多个 redis 实例，就增加 instances 即可：

```toml
[[instances]]
address = "10.23.25.2:6379"
username = ""
password = ""
labels = { instance="n9e-10.23.25.2:6379" }

[[instances]]
address = "10.23.25.3:6379"
username = ""
password = ""
labels = { instance="n9e-10.23.25.3:6379" }
```

建议通过 labels 配置附加一个 instance 标签，便于后面复用监控大盘。

## 监控大盘和告警规则

该 README 的同级目录下，提供了 dashboard.json 就是监控大盘的配置，alerts.json 是告警规则，可以导入夜莺使用。

