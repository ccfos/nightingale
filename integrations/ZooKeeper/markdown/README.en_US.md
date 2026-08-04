# zookeeper

ZooKeeper `>=3.6.0` has built-in [Prometheus support](https://zookeeper.apache.org/doc/current/zookeeperMonitor.html), but the
bundled dashboard queries the `zk_*` names emitted by Categraf's ZooKeeper
input. Use this input for the current dashboard. If native ZooKeeper metric
names differ, update the dashboard instead of only swapping the endpoint.

## Overview

The Categraf input uses ZooKeeper Four Letter Words commands. The real-data test
used ZooKeeper 3.9 and this input and passed all dashboard queries.

Note that zookeeper v3.4.10 and later added a whitelist for four-letter commands, so you need to add the whitelist setting to the zookeeper configuration file `zoo.cfg`:

```
4lw.commands.whitelist=mntr,ruok
```

In production, allow only the required commands instead of using `*`.

## Configuration

The zookeeper plugin configuration lives in `conf/input.zookeeper/zookeeper.toml`. Separate the addresses of multiple instances in a cluster with spaces:

```toml
[[instances]]
cluster_name = "dev-zk-cluster"
addresses = "127.0.0.1:2181"
timeout = 10
labels = { job = "zookeeper", instance = "zk-01:2181" }
```

To monitor multiple zookeeper clusters, just add more instances:

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

Run `./categraf --test --inputs zookeeper` and confirm `zk_up`,
`zk_znode_count`, and `zk_packets_sent`. Preserve the `job` and `instance`
labels used by the dashboard variables.
