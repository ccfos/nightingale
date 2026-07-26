# zookeeper

ZooKeeper `>=3.6.0` 内置 [Prometheus 支持](https://zookeeper.apache.org/doc/current/zookeeperMonitor.html)，但本目录的 `Zookeeper - exporter` 仪表盘查询的是 Categraf ZooKeeper input 生成的 `zk_*` 指标。要直接使用当前模板，应按下文启用 Categraf input。

如果改为抓取 ZooKeeper 原生 Prometheus 端点，需要先确认其指标名是否为 `zk_up`、`zk_znode_count`、`zk_packets_sent` 等；指标名不一致时必须同步修改模板，不能只替换采集端点。

## 说明

Categraf ZooKeeper input 移植于 [dabealu/zookeeper-exporter](https://github.com/dabealu/zookeeper-exporter)，通过 ZooKeeper 四字命令获取监控信息。本次使用 ZooKeeper 3.9 和该 input 完成 16/16 查询验证。

需要注意的是，在 zookeeper v3.4.10 以后添加了四字命令白名单，需要在 zookeeper 的配置文件 `zoo.cfg` 中新增白名单配置:

```
4lw.commands.whitelist=mntr,ruok
```

生产环境只开放采集所需命令，不要无条件配置为 `*`。

## 配置

zookeeper 插件的配置在 `conf/input.zookeeper/zookeeper.toml` 集群中的多个实例地址请用空格分隔：

```toml
[[instances]]
cluster_name = "dev-zk-cluster"
addresses = "127.0.0.1:2181"
timeout = 10
labels = { job = "zookeeper", instance = "zk-01:2181" }
```

如果要监控多个 zookeeper 集群，就增加 instances 即可：

```toml
[[instances]]
cluster_name = "dev-zk-cluster"
addresses = "127.0.0.1:2181"
timeout = 10
labels = { job = "zookeeper-a", instance = "zk-a-01:2181" }

[[instances]]
cluster_name = "test-zk-cluster"
addresses = "127.0.0.1:2181 127.0.0.1:2182 127.0.0.1:2183"
timeout = 10
labels = { job = "zookeeper-b" }
```

验证：

```bash
./categraf --test --inputs zookeeper
```

至少确认 `zk_up`、`zk_znode_count` 和 `zk_packets_sent`，并保留模板变量使用的 `job`、`instance` 标签。
