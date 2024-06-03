# Linux

Linux 类别下，包含多个内置插件，比如 cpu、mem、net、netstat、kernel_vmstat 等，这些插件大都是默认是开启的，无需额外配置，可能有额外配置需求的插件如下。

## cpu

统计 CPU 使用率，默认只采集整机的情况，不采集每个 CPU Core 的情况，如果想采集每个 CPU Core 的情况，可以配置如下。

```ini
collect_per_cpu = true
```

## netstat

统计网络连接数，默认配置如下，可根据实际情况调整。

```ini
# 默认开启了 smmary 统计，类似 ss -s 命令的输出
disable_summary_stats = false
# 默认关闭了所有连接的详细统计，在连接数较多的机器上统计此数据会影响性能
disable_connection_stats = true
# 读取 /proc/net/netstat 的内容，默认关闭了，可以开启，这部分不影响性能
tcp_ext = false
ip_ext = false
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