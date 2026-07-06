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

> 关键配置说明：
> 
> - endpoint: Categraf 监听端口，用于接收 Zabbix 推送的数据
> - instances.zabbix.server: Zabbix 服务器地址
> - instances.zabbix.version: Zabbix 版本
> - instances.zabbix.api_token: 上面创建的 API Token
> - instances.zabbix.name_as_tag: 是否将 Item 名称作为标签（调试时可启用）
> 
> 我们选的是 HTTP 方式实时导出，这里我们就只需要配置 endpoint 部分，相当于是 categraf 的 zabbix 插件会启动一个 9101 端口，用于接收 Zabbix 的数据推送。这里这个端口要和 Zabbix connector 的端口保持一致。Categraf 会通过 Zabbix API 获取 Item 详细信息，以便正确转换数据。推送的历史数据中，item 的 key_ 比 name 更适合直接作为指标名称，而 item 的单位、关联主机信息则用作标签，丰富指标的含义。

