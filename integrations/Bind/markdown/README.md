forked from [telegraf/snmp](https://github.com/influxdata/telegraf/tree/master/plugins/inputs/bind)

配置示例
```
[[instances]]
urls = [
 #"http://localhost:8053/xml/v3",
]

timeout = "5s"
gather_memory_contexts = true
gather_views = true
```