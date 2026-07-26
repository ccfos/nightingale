# TiDB

TiDB, PD, and TiKV expose Prometheus metrics through HTTP:

See the [TiDB Monitoring API](https://docs.pingcap.com/tidb/stable/tidb-monitoring-api/).

| Component | Default endpoint |
| --- | --- |
| TiDB Server | `http://<tidb>:10080/metrics` |
| PD Server | `http://<pd>:2379/metrics` |
| TiKV Server | `http://<tikv>:20180/metrics` |

The dashboard depends on `k8s_cluster`, `tidb_cluster`, `job`, and `instance`.
Set stable cluster labels even for a non-Kubernetes deployment:

```toml
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

Add every TiDB, PD, and TiKV node. Scrape TiFlash, TiCDC, and other optional
components only when deployed. The overview's `probe_success` panels also
require black-box probes with matching cluster and `group` labels.
