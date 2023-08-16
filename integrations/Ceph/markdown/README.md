# ceph plugin

开启 ceph prometheus 支持

```bash
ceph mgr module enable prometheus
```

## 采集配置

既然 ceph 可以暴露 prometheus 协议的 metrics 数据，则直接使用 prometheus 插件抓取即可。

categraf 配置文件：`conf/input.prometheus/prometheus.toml`

```yaml
[[instances]]
urls = [
  "http://192.168.11.181:9283/metrics"
]
labels = {service="ceph",cluster="ceph-cluster-001"}
```


## 仪表盘效果

夜莺内置仪表盘中已经内置了 ceph 的仪表盘，导入即可使用。

![20230801152445](https://download.flashcat.cloud/ulric/20230801152445.png)

## 告警规则

夜莺内置告警规则中已经内置了 ceph 的告警规则，导入即可使用。

![20230801152431](https://download.flashcat.cloud/ulric/20230801152431.png)