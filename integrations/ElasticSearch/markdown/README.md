# elasticsearch plugin

ElasticSearch 通过 HTTP JSON 的方式暴露了自身的监控指标，通过 categraf 的 [elasticsearch](https://github.com/flashcatcloud/categraf/tree/main/inputs/elasticsearch) 插件抓取。

如果是小规模集群，设置 `local=false`，从集群中某一个节点抓取数据，即可拿到整个集群所有节点的监控数据。如果是大规模集群，建议设置 `local=true`，在集群的每个节点上都部署抓取器，抓取本地 elasticsearch 进程的监控数据。

ElasticSearch 详细的监控讲解，请参考这篇 [文章](https://time.geekbang.org/column/article/628847)。

## 配置示例

categraf 配置文件：`conf/input.elasticsearch/elasticsearch.toml`

```yaml
[[instances]]
servers = ["http://192.168.11.177:9200"]
http_timeout = "10s"
local = false
cluster_health = true
cluster_health_level = "cluster"
cluster_stats = true
indices_level = ""
node_stats = ["jvm", "breaker", "process", "os", "fs", "indices", "thread_pool", "transport"]
username = "elastic"
password = "xxxxxxxx"
num_most_recent_indices = 1
labels = { service="es" }
```

## 仪表盘效果

夜莺内置仪表盘中已经内置了 Elasticsearch 的仪表盘，导入即可使用。

![](http://download.flashcat.cloud/uPic/es-dashboard.jpeg)