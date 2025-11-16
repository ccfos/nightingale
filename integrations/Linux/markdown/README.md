# Linux

Linux 集成是 Nightingale 的核心基础组件之一。它通过 Categraf 采集器，自动收集 Linux 操作系统的各项关键指标，如 CPU、内存、磁盘、网络、系统负载等，帮助您快速搭建主机监控能力。Linux 类别下，包含多个内置插件，比如 cpu、mem、net、netstat、kernel_vmstat 等，这些插件大都是默认是开启的，无需额外配置，可能有额外配置需求的插件如下。

## cpu

统计 CPU 使用率，默认只采集整机的情况，不采集每个 CPU Core 的情况，如果想采集每个 CPU Core 的情况，可以配置如下。

```ini
collect_per_cpu = true
```

## netstat

统计网络连接数，默认配置如下，可根据实际情况调整。

```ini
# 默认开启了 summary 统计，类似 ss -s 命令的输出
disable_summary_stats = false
# 默认关闭了所有连接的详细统计，在连接数较多的机器上统计此数据会影响性能
disable_connection_stats = true
# 读取 /proc/net/netstat 的内容，默认关闭了，可以开启，这部分不影响性能
tcp_ext = false: 是否采集 /proc/net/snmp 中 TCP 相关的详细指标（如 TcpExtListenOverflows 等）。
ip_ext = false: 是否采集 /proc/net/snmp 中 IP 相关的详细指标（如 IpExtInOctets 等）。
```

## disk

统计磁盘使用率，默认配置如下，可根据实际情况调整。

```ini
# 严格指定要采集的挂载点，如果指定了，就只采集指定的挂载点
# mount_points = ["/"]

# 有些 fstype 没必要采集，可以忽略
ignore_fs = ["tmpfs", "devtmpfs", "devfs", "iso9660", "overlay", "aufs", "squashfs", "nsfs", "CDFS", "fuse.juicefs"]

# 有些挂载点没必要采集，可以忽略，这里可以配置前缀，符合前缀的挂载点都会被忽略
ignore_mount_points = ["/boot", "/var/lib/kubelet/pods"]
```

## kernel_vmstat

统计的信息来自 `/proc/vmstat`，只有高版本内核才支持，这个文件的内容较多，默认配置只采集了 oom_kill 次数，其他指标均未采集，如果你想打开其他采集开关，可以修改 white_list 部分的配置。下面是截取了一部分内容，供参考：

```toml
[white_list]
oom_kill = 1
nr_free_pages = 0
nr_alloc_batch = 0
...
```

## arp_package

统计 ARP 包的数量，该插件依赖 cgo，如果需要该插件需要下载 `with-cgo` 的 categraf 发布包。


## ntp

监控机器时间偏移量，只需要给出 ntp 服务端地址，Categraf 就会周期性去请求，对比本机时间，得到偏移量，监控指标是 ntp_offset_ms 顾名思义，单位是毫秒，一般这个值不能超过 1000

## 仪表盘与告警

本集成默认内置了丰富的仪表盘和告警规则，帮助您快速实现 Linux 监控告警。

### 内置仪表盘

* `Linux Host Overview by Categraf`：提供核心指标的概览视图。
* `Linux Host Detail by Categraf`：提供更详细的主机性能指标。

### 内置告警规则

本集成提供了两套告警规则，您可按需导入：

* `linux_by_categraf.json`：包含磁盘、内存、网络丢包等基础告警。
* `CommonAlertingRules-Categraf.json`：包含一套更丰富、更通用的主机告警规则（推荐）。