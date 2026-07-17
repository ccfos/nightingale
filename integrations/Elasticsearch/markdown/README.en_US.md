# elasticsearch plugin

ElasticSearch exposes its own monitoring metrics over HTTP JSON, which are scraped by the categraf [elasticsearch](https://github.com/flashcatcloud/categraf/tree/main/inputs/elasticsearch) plugin.

For small clusters, set `local=false` and scrape any single node in the cluster to get monitoring data for all nodes. For large clusters, it is recommended to set `local=true` and deploy a scraper on every node in the cluster, each collecting monitoring data from its local elasticsearch process.


## Configuration Example

categraf configuration file: `conf/input.elasticsearch/elasticsearch.toml`

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
