# zookeeper

Note: zookeeper versions `>=3.6.0` have built-in [prometheus support](https://zookeeper.apache.org/doc/current/zookeeperMonitor.html). That is, if prometheus is enabled in zookeeper, Categraf can simply pull data from that metrics endpoint using the prometheus plugin, and there is no need to use this zookeeper plugin for collection.

## Overview

The categraf zookeeper collection plugin is ported from [dabealu/zookeeper-exporter](https://github.com/dabealu/zookeeper-exporter) and is intended for zookeeper versions `<3.6.0`. It works by using ZooKeeper's Four Letter Words commands to retrieve monitoring information.

Note that zookeeper v3.4.10 and later added a whitelist for four-letter commands, so you need to add the whitelist setting to the zookeeper configuration file `zoo.cfg`:

```
4lw.commands.whitelist=mntr,ruok
```

## Configuration

The zookeeper plugin configuration lives in `conf/input.zookeeper/zookeeper.toml`. Separate the addresses of multiple instances in a cluster with spaces:

```toml
[[instances]]
cluster_name = "dev-zk-cluster"
addresses = "127.0.0.1:2181"
timeout = 10
```

To monitor multiple zookeeper clusters, just add more instances:

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
