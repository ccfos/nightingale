## zabbix

## Configuration

```toml
[[items]]
……


[[instances]]
endpoint=":9101"

[instances.zabbix]
server="http://192.168.10.222"
version="7.2"
api_token="xxxxx"
##name_as_tag=true
```

> Key configuration notes:
> 
> - endpoint: the port Categraf listens on to receive data pushed from Zabbix
> - instances.zabbix.server: the Zabbix server address
> - instances.zabbix.version: the Zabbix version
> - instances.zabbix.api_token: the API Token created above
> - instances.zabbix.name_as_tag: whether to use the Item name as a tag (can be enabled for debugging)
> 
> We chose the HTTP real-time export approach, so we only need to configure the endpoint part. Essentially, the zabbix plugin of categraf starts listening on port 9101 to receive data pushed from Zabbix. This port must match the port configured in the Zabbix connector. Categraf fetches Item details via the Zabbix API in order to convert the data correctly. In the pushed history data, an item's key_ is better suited than its name to serve directly as the metric name, while the item's unit and associated host information are used as labels to enrich the meaning of the metric.
