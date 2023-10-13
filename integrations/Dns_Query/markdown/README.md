# 应用场景
一般用于对DNS服务器的响应监测，帮助运维快速定位网络问题。

# 部署场景
不需要每台虚拟机都启用此插件，建议是独立或复合的某一台虚拟机启用此插件。

# 配置场景
```
本配置启用或数据定义如下功能：
使用本机DNS查询域名解析质量。
使用外部DNS查询域名解析质量。
使用不同记录类型进行DNS查询。
每种查询都设置超时时间5秒。
增加自定义标签，可通过自定义标签筛选数据及更加精确的告警推送。
在domains字段处增加自己想要被DNS查询的域名，一般填写公司业务系统的域名或第三方依赖的业务系统。
```

# 修改dns_query.toml文件配置

``` 以下文件内容配置作为参考
[root@aliyun input.dns_query]# cat dns_query.toml
# # collect interval
# interval = 15

[[instances]]
# # append some labels for series
labels = { cloud="huaweicloud", region="huabei-beijing-4",azone="az1", product="n9e" }

# # interval = global.interval * interval_times
# interval_times = 1

# #
auto_detect_local_dns_server  = true

### A record

## servers to query
servers = ["223.5.5.5","114.114.114.114","119.29.29.29"]

## Network is the network protocol name.
# network = "udp"

## Domains or subdomains to query.
domains = ["www.huaweicloud.com", "www.baidu.com", "www.tapd.cn"]

## Query record type.
## Possible values: A, AAAA, CNAME, MX, NS, PTR, TXT, SOA, SPF, SRV.
record_type = "A"

## Dns server port.
# port = 53

## Query timeout in seconds.
timeout = 5


### CNAME record

[[instances]]
# # append some labels for series
labels = { cloud="huaweicloud", region="huabei-beijing-4",azone="az1", product="n9e" }

# # interval = global.interval * interval_times
# interval_times = 1

# #
auto_detect_local_dns_server  = false

## servers to query
servers = ["223.5.5.5","114.114.114.114","119.29.29.29"]

## Network is the network protocol name.
# network = "udp"

## Domains or subdomains to query.
domains = ["www.huaweicloud.com", "www.baidu.com", "www.tapd.cn"]

## Query record type.
## Possible values: A, AAAA, CNAME, MX, NS, PTR, TXT, SOA, SPF, SRV.
record_type = "CNAME"

## Dns server port.
# port = 53

## Query timeout in seconds.
timeout = 5


### NS record

[[instances]]
# # append some labels for series
labels = { cloud="huaweicloud", region="huabei-beijing-4",azone="az1", product="n9e" }

# # interval = global.interval * interval_times
# interval_times = 1

# #
auto_detect_local_dns_server  = false

## servers to query
servers = ["223.5.5.5","114.114.114.114","119.29.29.29"]

## Network is the network protocol name.
# network = "udp"

## Domains or subdomains to query.
domains = ["www.huaweicloud.com", "www.baidu.com", "www.tapd.cn"]

## Query record type.
## Possible values: A, AAAA, CNAME, MX, NS, PTR, TXT, SOA, SPF, SRV.
record_type = "NS"

## Dns server port.
# port = 53

## Query timeout in seconds.
timeout = 5
```

# 测试配置
```
./categraf --test --inputs dns_query
....... A记录同理就省略
20:51:34 dns_query_rcode_value agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud domain=www.tapd.cn product=n9e record_type=CNAME region=huabei-beijing-4 server=119.29.29.29 0
20:51:34 dns_query_result_code agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud domain=www.tapd.cn product=n9e record_type=CNAME region=huabei-beijing-4 server=119.29.29.29 0
20:51:34 dns_query_query_time_ms agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud domain=www.tapd.cn product=n9e record_type=CNAME region=huabei-beijing-4 server=119.29.29.29 33.500371

20:51:34 dns_query_rcode_value agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud domain=www.baidu.com product=n9e record_type=CNAME region=huabei-beijing-4 server=119.29.29.29 0
20:51:34 dns_query_result_code agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud domain=www.baidu.com product=n9e record_type=CNAME region=huabei-beijing-4 server=119.29.29.29 0
20:51:34 dns_query_query_time_ms agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud domain=www.baidu.com product=n9e record_type=CNAME region=huabei-beijing-4 server=119.29.29.29 34.328242

20:51:34 dns_query_rcode_value agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud domain=www.huaweicloud.com product=n9e record_type=CNAME region=huabei-beijing-4 server=119.29.29.29 0
20:51:34 dns_query_result_code agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud domain=www.huaweicloud.com product=n9e record_type=CNAME region=huabei-beijing-4 server=119.29.29.29 0
20:51:34 dns_query_query_time_ms agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud domain=www.huaweicloud.com product=n9e record_type=CNAME region=huabei-beijing-4 server=119.29.29.29
.....

```
# 重启服务
```
重启categraf服务生效
systemctl daemon-reload && systemctl restart categraf && systemctl status categraf

查看启动日志是否有错误
journalctl -f -n 500 -u categraf | grep "E\!" | grep "W\!"
```

# 检查数据呈现
等待1-2分钟后数据就会在图表中展示出来，如图：
![image](https://user-images.githubusercontent.com/12181410/220353480-e17a7822-7ccc-4fdf-b18b-a0be84cd5550.png)

# 监控告警规则配置
```
个人经验仅供参考，一般DNS解析延迟时间：
超过2000毫秒，为P2级别，启用企业微信应用推送告警，3分钟内恢复发出恢复告警。
超过5000毫秒，为P1级别，启用电话语音告警&企业微信应用告警，3分钟内恢复发出恢复告警。

为什么会这么考量设计？
在用到DNS监控时，一般公司业务是遍布全国的，然而全国各个地区在解析DNS存在各种场景因素导致的DNS问题（如DNS被劫持、片区DNS服务器故障等），所以需要以高级别对待。
从收到告警到恢复告警设置3分钟的意图是防止期间是短暂时间有问题,同时也给SLA(99.99%)给足处理时长。
```

# 监控图表配置
```
先略过
```
