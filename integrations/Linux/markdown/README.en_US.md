# Linux

The Linux category contains multiple built-in plugins, such as cpu, mem, net, netstat, kernel_vmstat, etc. Most of these plugins are enabled by default and require no extra configuration. The plugins that may need additional configuration are listed below.

## cpu

Collects CPU usage. By default only the overall machine usage is collected, not per-CPU-core usage. If you want to collect per-core metrics, configure it as follows.

```ini
collect_per_cpu = true
```

## netstat

Collects network connection statistics. The default configuration is shown below and can be adjusted as needed.

```ini
# Summary statistics are enabled by default, similar to the output of the ss -s command
disable_summary_stats = false
# Detailed statistics for all connections are disabled by default; collecting this data on machines with many connections can hurt performance
disable_connection_stats = true
# Read the contents of /proc/net/netstat; disabled by default, can be enabled, this part does not affect performance
tcp_ext = false
ip_ext = false
```

## disk

Collects disk usage. The default configuration is shown below and can be adjusted as needed.

```ini
# Strictly specify the mount points to collect; if specified, only these mount points will be collected
# mount_points = ["/"]

# Some fstypes are not worth collecting and can be ignored
ignore_fs = ["tmpfs", "devtmpfs", "devfs", "iso9660", "overlay", "aufs", "squashfs", "nsfs", "CDFS", "fuse.juicefs"]

# Some mount points are not worth collecting and can be ignored; prefixes can be configured here, and any mount point matching a prefix will be ignored
ignore_mount_points = ["/boot", "/var/lib/kubelet/pods"]
```

## kernel_vmstat

The statistics come from `/proc/vmstat`, which is only supported on newer kernels. This file contains a lot of entries; by default only the oom_kill count is collected, and no other metrics are collected. If you want to enable collection of other metrics, modify the white_list section of the configuration. Below is an excerpt for reference:

```toml
[white_list]
oom_kill = 1
nr_free_pages = 0
nr_alloc_batch = 0
...
```

## arp_package

Collects the number of ARP packets. This plugin depends on cgo; if you need it, download the `with-cgo` categraf release package.


## ntp

Monitors the machine's time offset. You only need to provide the NTP server address, and Categraf will periodically query it, compare with the local time, and compute the offset. The metric is ntp_offset_ms, which, as the name suggests, is in milliseconds. Generally this value should not exceed 1000.
