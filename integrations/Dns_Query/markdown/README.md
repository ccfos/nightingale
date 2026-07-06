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

# 配置说明
配置文件在 conf/input.dns_query/dns_query.toml。
```toml
[[instances]]
auto_detect_local_dns_server = false # 是否自动检测本地 DNS 服务器。当设置为 true 时，插件会从 /etc/resolv.conf 文件中读取本地配置的 DNS 服务器

servers = ["114.114.114.114"] # 要查询的 DNS 服务器列表

network = "udp" # 查询使用的协议 udp还是tcp

domains = ["flashcat.cloud"] # 要查询的域名列表

record_type="NS" # DNS 查询的记录类型

port = 53 # DNS 服务器端口号

timeout = 2 DNS 查询超时时间（秒）

expect_query_ips = { "域名" = ["IP1", "IP2", ...] } # 用于域名劫持检测的期望 IP 地址配置
```
1. record_type 支持以下类型：

- A：IPv4 地址记录，返回域名对应的 IPv4 地址
- AAAA：IPv6 地址记录，返回域名对应的 IPv6 地址
- CNAME：别名记录，返回域名的规范名称
- MX：邮件交换记录，返回邮件服务器信息
- NS：名称服务器记录，返回域名的权威 DNS 服务器
- PTR：指针记录，用于反向 DNS 查询（IP 地址到域名）
- TXT：文本记录，返回与域名关联的文本信息
- SOA：起始授权机构记录，返回域名的权威信息
- SPF：发送方策略框架记录，用于邮件防伪
- SRV：服务记录，指定服务的位置信息
- ANY：任意记录类型，返回所有可用的记录

2. expect_query_ips 需要满足一下条件
- 只有当 record_type 为 A 或 AAAA 时才有效
- IP 列表应该尽可能完整，包含该域名的所有合法 IP 地址
- 建议定期更新 IP 列表以确保准确性


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

# 指标说明
1. dns_query_time_ms 表示查询的响应时间，单位ms;
2. dns_query_result_value是categraf自定义的查询结果，0表示成功，1表示超时，2表示出错了。
3. dns_query_rcode_value 是 DNS 响应中的状态码，表示查询请求的处理结果：
    - 0 (NOERROR)：查询成功，没有错误
    - 1 (FORMERR)：格式错误，DNS 服务器无法理解查询请求
    - 2 (SERVFAIL)：服务器失败，DNS 服务器遇到内部错误
    - 3 (NXDOMAIN)：域名不存在
    - 4 (NOTIMP)：查询类型不被支持
    - 5 (REFUSED)：查询被拒绝，通常是策略原因 在监控中，RCODE 为 0 表示查询成功，其他值通常表示存在问题需要关注
4. expect_query_ips配置了对应的域名后， categraf会对比响应的IP列表与配置的IP列表, 此时会产生新的指标 dns_query_status_change, 0 表示响应的IP与期望的IP一致， 1表示响应的IP与期望的IP出现差异。 当出现差异时，会生成新的指标 dns_query_status_change_detail{ips="响应的IP列表", diff="响应IP列表-期望IP列表"} 1
```toml
dns_query_status_change{agent_hostname="localhost",domain="baidu.com",record_type="A",server="114.114.114.114"} 1
dns_query_status_change_detail{agent_hostname="localhost",diff="182.61.201.211",domain="baidu.com",ips="182.61.201.211,182.61.244.181",record_type="A",server="114.114.114.114"} 1
```

# 监控告警规则配置
```
个人经验仅供参考，一般DNS解析延迟时间：
超过2000毫秒，为P2级别，启用企业微信应用推送告警，3分钟内恢复发出恢复告警。
超过5000毫秒，为P1级别，启用电话语音告警&企业微信应用告警，3分钟内恢复发出恢复告警。

为什么会这么考量设计？
在用到DNS监控时，一般公司业务是遍布全国的，然而全国各个地区在解析DNS存在各种场景因素导致的DNS问题（如DNS被劫持、片区DNS服务器故障等），所以需要以高级别对待。
从收到告警到恢复告警设置3分钟的意图是防止期间是短暂时间有问题,同时也给SLA(99.99%)给足处理时长。
```

