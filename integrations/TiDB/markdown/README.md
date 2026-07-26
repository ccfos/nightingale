# TiDB

TiDB、PD 和 TiKV 都通过 HTTP `/metrics` 暴露 Prometheus 指标。本次实测模板已查询到 TiDB 集群真实数据，但单套测试拓扑没有覆盖 TiFlash、TiCDC、Pump、Drainer 和所有异常场景。

参考：[TiDB Monitoring API](https://docs.pingcap.com/tidb/stable/tidb-monitoring-api/)。

## 核心指标端点

| 组件 | 默认端点 |
| --- | --- |
| TiDB Server | `http://<tidb>:10080/metrics` |
| PD Server | `http://<pd>:2379/metrics` |
| TiKV Server | `http://<tikv>:20180/metrics` |

如果部署了 TiFlash、TiCDC 或其他组件，还需要抓取其实际配置的 metrics 端点。

## Categraf 配置

当前仪表盘依赖 `k8s_cluster`、`tidb_cluster`、`job` 和 `instance` 标签。非 Kubernetes 部署也需要为前两个标签设置稳定值：

```toml
# conf/input.prometheus/tidb.toml
interval = 15

[[instances]]
urls = ["http://tidb-01:10080/metrics"]
url_label_key = "instance"
url_label_value = "{{.Host}}"
labels = { job = "tidb", tidb_cluster = "prod-tidb", k8s_cluster = "baremetal" }

[[instances]]
urls = ["http://pd-01:2379/metrics"]
url_label_key = "instance"
url_label_value = "{{.Host}}"
labels = { job = "pd", tidb_cluster = "prod-tidb", k8s_cluster = "baremetal" }

[[instances]]
urls = ["http://tikv-01:20180/metrics"]
url_label_key = "instance"
url_label_value = "{{.Host}}"
labels = { job = "tikv", tidb_cluster = "prod-tidb", k8s_cluster = "baremetal" }
```

多节点集群应为每个 TiDB、PD 和 TiKV 实例增加一个 `[[instances]]`，三类组件的 `tidb_cluster` 和 `k8s_cluster` 必须一致。

## 验证

```bash
curl -fsS http://tidb-01:10080/metrics | grep tidb_executor_statement_total | head
curl -fsS http://pd-01:2379/metrics | grep pd_cluster_status | head
curl -fsS http://tikv-01:20180/metrics | grep tikv_grpc_msg_duration | head
```

模板顶部的组件存活总览还使用 `probe_success`。如果需要这些面板，应额外用黑盒探测采集各端点，并附加相同的 `k8s_cluster`、`tidb_cluster` 以及对应 `group` 标签。没有部署 TiFlash、TiCDC、Pump 或 Drainer 时，其面板为空属于拓扑差异。
