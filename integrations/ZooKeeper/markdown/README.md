# zookeeper
**注意: >=3.6.0 zookeeper 版本内置 [prometheus 的支持](https://zookeeper.apache.org/doc/current/zookeeperMonitor.html)，即，如果 zookeeper 启用了 prometheus，Categraf 可使用 prometheus 插件从这个 metrics 接口拉取数据即可;**

移植于 [dabealu/zookeeper-exporter](https://github.com/dabealu/zookeeper-exporter)，适用于 `<3.6.0` 版本的 zookeeper, 原理就是利用 Zookeper 提供的四字命令（The Four Letter Words）获取监控信息；

需要注意的是，在 zookeeper v3.4.10 以后添加了四字命令白名单，需要在 zookeeper 的配置文件 `zoo.cfg` 中新增白名单配置:
```
4lw.commands.whitelist=mntr,ruok
```

## Configuration

zookeeper 插件的配置在 `conf/input.zookeeper/zookeeper.toml` 集群中的多个实例地址请用空格分隔：

```toml
[[instances]]
cluster_name = "dev-zk-cluster"
addresses = "127.0.0.1:2181"
timeout = 10
```

如果要监控多个 zookeeper 集群，就增加 instances 即可：

```toml
[[instances]]
cluster_name = "dev-zk-cluster"
addresses = "127.0.0.1:2181"
timeout = 10

[[instances]]
cluster_name = "test-zk-cluster"
addresses = "127.0.0.1:2181 127.0.0.1:2182 127.0.0.1:2183"
timeout = 10
```

## 监控大盘和告警规则

该 README 的同级目录下，提供了 dashboard.json 就是监控大盘的配置，alerts.json 是告警规则，可以导入夜莺使用。

