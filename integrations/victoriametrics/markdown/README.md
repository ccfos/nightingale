## VictoriaMetrics Dashboard & Alerts

使用[categraf](https://github.com/flashcatcloud/categraf)中[inputs.prometheus](https://github.com/flashcatcloud/categraf/tree/main/inputs/prometheus)插件采集[VictoriaMetrics](https://docs.victoriametrics.com/)三个服务组件默认暴露的指标数据:

写入模块： `vminsert` 端口：`8480` URI：`metrics`

查询模块： `vmselect` 端口：`8481` URI：`metrics`

存储模块： `vmstorage` 端口：`8482` URI：`metrics`

### 配置文件示例：

分为俩个Dashboard：

1. 其中label_key: `instance` ，label: `service`  为[dashboard](../dashboard/victoriametrics_cluster_ig1.json)中选择变量，当时制作的版本是v1.34.0，很多指标已经不适配高版本，不推荐使用，官方于2020-03-09后不在进行维护；[Victoria Metrics cluster - IG1 version](https://grafana.com/grafana/dashboards/11831-victoria-metrics-cluster-ig1-version/)

2. 其中label_key: `instance` ，label: `job`  为[dashboard](../dashboard/victoriametrics-cluster.json)中选择变量，制作版本为v1.83.0，已经在1.90.0进行过验证，理论适配当前所有迭代版本，所有指标描述调整为中文，这个仪表盘为官方推荐的集群仪表盘，一直在持续更新，推荐使用这个；[VictoriaMetrics - cluster](https://grafana.com/grafana/dashboards/11176-victoriametrics-cluster/)；

```toml
# vmstorage
[[instances]]
urls = [
     "http://127.0.0.1:8482/metrics"
]
url_label_key = "instance"
url_label_value = "{{.Host}}"
labels = {service="vmstorage"}

# vmselect
[[instances]]
urls = [
     "http://127.0.0.1:8481/metrics"
]

url_label_key = "instance"
url_label_value = "{{.Host}}"
labels = {service="vmselect"}

# vminsert
[[instances]]
urls = [
     "http://127.0.0.1:8480/metrics"
]
url_label_key = "instance"
url_label_value = "{{.Host}}"
labels = {service="vminsert"}
```

### 告警规则

[alerts](../alerts/alerts.json)
