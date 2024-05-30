# Linux

categraf 部署之后，就会自动采集 CPU、内存、磁盘、IO、网路相关的指标，无需额外配置。

## 内置仪表盘

夜莺内置了仪表盘，文件名是 `_categraf` 的表示是使用 categraf 作为采集器。文件名是 `_exporter` 的表示是使用 node-exporter 作为采集器。

## 内置告警规则

夜莺内置了告警规则，文件名是 `_categraf` 的表示是使用 categraf 作为采集器。文件名是 `_exporter` 的表示是使用 node-exporter 作为采集器。

下面是一个可自己配置开启的插件
## arp packet
### 调整间隔时间
如有诉求对此插件本身的采集间隔时间调整的话就启用,单位为秒
interval = 15

### 获取被监控端设备的网卡名称
可用以下命令获取网卡名称列表
```
ip addr | grep '^[0-9]' |awk -F':' '{print $2}'

 lo
 eth0
 br-153e7f4f0c83
 br-2f302c2a8faa
 br-5ae0cdb82efc
 br-68cba8773a8c
 br-c50ca3122079
 docker0
 br-fd769e4347bd
 veth944ac75@if52
```
### 在数组instances中启用eth_device
将以上获取的网卡列表，根据自己的诉求填入，如eth0
```
eth_device="eth0"
```
### 测试是否能获取到值
```
./categraf --test --inputs arp_packet

```

## netstat
该插件采集网络连接情况，比如有多少 time_wait 连接，多少 established 连接

## kernel_vmstat
该监控插件采集的是 `/proc/vmstat` 的指标数据，需要较高版本的 kernel，`/proc/vmstat`内容较多，配置文件中给了一个白名单的配置，大家按需启用，只有启用了才会采集。

```ini
[white_list]
oom_kill = 1
nr_free_pages = 0
nr_alloc_batch = 0
nr_inactive_anon = 0
nr_active_anon = 0
nr_inactive_file = 0
nr_active_file = 0
nr_unevictable = 0
nr_mlock = 0
nr_anon_pages = 0
nr_mapped = 0
nr_file_pages = 0
nr_dirty = 0
nr_writeback = 0
nr_slab_reclaimable = 0
nr_slab_unreclaimable = 0
nr_page_table_pages = 0
nr_kernel_stack = 0
nr_unstable = 0
nr_bounce = 0
nr_vmscan_write = 0
nr_vmscan_immediate_reclaim = 0
nr_writeback_temp = 0
nr_isolated_anon = 0
nr_isolated_file = 0
nr_shmem = 0
nr_dirtied = 0
nr_written = 0
numa_hit = 0
numa_miss = 0
numa_foreign = 0
numa_interleave = 0
numa_local = 0
numa_other = 0
workingset_refault = 0
workingset_activate = 0
workingset_nodereclaim = 0
nr_anon_transparent_hugepages = 0
nr_free_cma = 0
nr_dirty_threshold = 0
nr_dirty_background_threshold = 0
pgpgin = 0
pgpgout = 0
pswpin = 0
pswpout = 0
pgalloc_dma = 0
pgalloc_dma32 = 0
pgalloc_normal = 0
pgalloc_movable = 0
pgfree = 0
pgactivate = 0
pgdeactivate = 0
pgfault = 0
pgmajfault = 0
pglazyfreed = 0
pgrefill_dma = 0
pgrefill_dma32 = 0
pgrefill_normal = 0
pgrefill_movable = 0
pgsteal_kswapd_dma = 0
pgsteal_kswapd_dma32 = 0
pgsteal_kswapd_normal = 0
pgsteal_kswapd_movable = 0
pgsteal_direct_dma = 0
pgsteal_direct_dma32 = 0
pgsteal_direct_normal = 0
pgsteal_direct_movable = 0
pgscan_kswapd_dma = 0
pgscan_kswapd_dma32 = 0
pgscan_kswapd_normal = 0
pgscan_kswapd_movable = 0
pgscan_direct_dma = 0
pgscan_direct_dma32 = 0
pgscan_direct_normal = 0
pgscan_direct_movable = 0
pgscan_direct_throttle = 0
zone_reclaim_failed = 0
pginodesteal = 0
slabs_scanned = 0
kswapd_inodesteal = 0
kswapd_low_wmark_hit_quickly = 0
kswapd_high_wmark_hit_quickly = 0
pageoutrun = 0
allocstall = 0
pgrotated = 0
drop_pagecache = 0
drop_slab = 0
numa_pte_updates = 0
numa_huge_pte_updates = 0
numa_hint_faults = 0
numa_hint_faults_local = 0
numa_pages_migrated = 0
pgmigrate_success = 0
pgmigrate_fail = 0
compact_migrate_scanned = 0
compact_free_scanned = 0
compact_isolated = 0
compact_stall = 0
compact_fail = 0
compact_success = 0
htlb_buddy_alloc_success = 0
htlb_buddy_alloc_fail = 0
unevictable_pgs_culled = 0
unevictable_pgs_scanned = 0
unevictable_pgs_rescued = 0
unevictable_pgs_mlocked = 0
unevictable_pgs_munlocked = 0
unevictable_pgs_cleared = 0
unevictable_pgs_stranded = 0
thp_fault_alloc = 0
thp_fault_fallback = 0
thp_collapse_alloc = 0
thp_collapse_alloc_failed = 0
thp_split = 0
thp_zero_page_alloc = 0
thp_zero_page_alloc_failed = 0
balloon_inflate = 0
balloon_deflate = 0
balloon_migrate = 0
```


# processes

如果进程总量太多，比如超过了 CPU core 的 3 倍，就需要关注了。

## 配置说明

configuration file: `conf/input.processes/processes.toml`

默认配置如下（一般维持默认不用动）：

```toml
# # collect interval
# interval = 15

# # force use ps command to gather
# force_ps = false

# # force use /proc to gather
# force_proc = false
```

有两种采集方式，使用 ps 命令，或者直接读取 `/proc` 目录，默认是后者。如果想强制使用 ps 命令才采集，开启 force_ps 即可：

```toml
force_ps = true
```