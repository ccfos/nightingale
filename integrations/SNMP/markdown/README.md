# snmp

> 监控网络设备，主要是通过 SNMP 协议，Categraf、Telegraf、Datadog-Agent、snmp_exporter 都提供了这个能力。

Categraf 从 v0.2.13 版本开始把 Telegraf 的 snmp 插件集成了进来，推荐大家采用这个插件来监控网络设备。这个插件的核心逻辑是：要采集什么指标，直接配置对应的 oid 即可，而且可以把一些 oid 采集到的数据当做时序数据的标签，非常非常灵活。

当然，弊端也有，因为 SNMP 体系里有大量的私有 oid，比如不同的设备获取 CPU、内存利用率的 oid 都不一样，这就需要为不同的型号的设备采用不同的配置，维护起来比较麻烦，需要大量的积累。这里我倡议大家把不同的设备型号的采集配置积累到 [这里](https://github.com/flashcatcloud/categraf/tree/main/inputs/snmp)，每个型号一个文件夹，长期积累下来，那将是利人利己的好事。不知道如何提 PR 的可以联系我们。

另外，也不用太悲观，针对网络设备而言，大部分监控数据的采集都是通用 oid 就可以搞定的，举个例子：

```toml
interval = 120

[[instances]]
agents = ["udp://172.30.15.189:161"]

interval_times = 1
timeout = "5s"
version = 2
community = "public"
agent_host_tag = "switch_ip"
retries = 1

[[instances.field]]
oid = "RFC1213-MIB::sysUpTime.0"
name = "uptime"

[[instances.field]]
oid = "RFC1213-MIB::sysName.0"
name = "source"
is_tag = true

[[instances.table]]
oid = "IF-MIB::ifTable"
name = "interface"
inherit_tags = ["source"]

[[instances.table.field]]
oid = "IF-MIB::ifDescr"
name = "ifDescr"
is_tag = true

```

上面的样例是 v2 版本的配置，如果是 v3 版本，校验方式举例：

```toml
version = 3
sec_name = "managev3user"
auth_protocol = "SHA"
auth_password = "example.Demo.c0m"
```

另外，snmp 的采集，建议大家部署单独的 Categraf 来做，因为不同监控对象采集频率可能不同，比如边缘交换机，我们 5min 采集一次就够了，核心交换机可以配置的频繁一些，比如 60s 或者 120s。

> 注意：如果采集的过于频繁，有些老款的交换机可能会被打挂，或者被限流，被限流的结果就是图上看到的是断点。

## 扩展阅读

- [SNMP(简单网络管理协议)简介](https://flashcat.cloud/blog/snmp-introduction/)
- [SNMP命令相关参数介绍](https://flashcat.cloud/blog/snmp-command-arguments/)
- [通过 Categraf SNMP 插件采集监控数据](https://flashcat.cloud/blog/snmp-metrics-collect-by-categraf/)

## 排错

要想通过 categraf 采集到 snmp 数据，首先要保证 categraf 所在的机器能够连通网络设备，可以通过 snmpget 命令来做测试：

```bash
snmpget -v2c -c public 172.30.15.189 RFC1213-MIB::sysUpTime.0
```

如果 snmpget 都跑不通，就得先解决这个问题，比如是 snmpd 没有启动，或者防火墙限制了 snmp 的访问，还是 snmpget 命令没有安装，等等。这些问题，gpt 和 google 都可以解决，这里不再赘述。
