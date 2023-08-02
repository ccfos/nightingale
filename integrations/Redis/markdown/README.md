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

夜莺内置了 redis 的告警规则和监控大盘，克隆到自己的业务组下即可使用。

## redis 集群如何监控

其实，redis 集群的监控，还是去监控每个 redis 实例。

如果一个 redis 集群有 3 个实例，对于业务应用来讲，发起一个请求，可能随机请求到某一个实例上去了，这个是没问题的，但是对于监控 client 而言，显然是希望到所有实例上获取数据的。

当然，如果多个 redis 实例组成了集群，我们希望有个标识来标识这个集群，这个时候，可以通过 labels 来实现，比如给每个实例增加一个 redis_clus 的标签，值为集群名字即可。
