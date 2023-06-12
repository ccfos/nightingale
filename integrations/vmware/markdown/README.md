## Vmware vSphere Dashboard & Alerts

使用[categraf](https://github.com/flashcatcloud/categraf)中[inputs.vsphere](https://github.com/flashcatcloud/categraf/tree/main/inputs/vsphere)插件采集Vmware指标数据:

VMware vSphere的两个核心组件： ESXi Server & vCenter Server

ESXi Server是Hypervsior，在其中创建和运行虚拟机和虚拟设备。

vCenter Server是用于管理网络中连接的多个ESXi主机和主机资源池的服务。

`想实现监控vsphere套间，必须部署vcenter.`

完整的部署vsphere&n9e&categraf采集文档：[夜莺监控之Categraf监控VMwareSphere](https://unixsre.com/posts/n9e-monitor-vsphere/)

### Categraf中conf/input.vsphere/vsphere.toml配置文件：

只需要修改其中的vcenter地址与username和password就可以使用，默认账号是administrator，权限较大，仅做测试使用，建议做权限做控制，可以在vcenter中自己建用户跟角色；

```toml
# # collect interval
# interval = 15

# Read metrics from one or many vCenters
[[instances]]
  labels = { instance="192.168.11.111", clustername="Datacenter" }
  ##  vCenter URLs to be monitored. These three lines must be uncommented
  ## and edited for the plugin to work.
  ## FQDN URLs to be monitored. These three lines must be uncommented
  vcenter = "https://vcenter.unixsre.com/sdk"
  username = "administrator@vcenter.unixsre.com"
  password = "111111119@abcdef"

  ## VMs
  ## Typical VM metrics (if omitted or empty, all metrics are collected)
  # vm_include = [ "/*/vm/**"] # Inventory path to VMs to collect (by default all are collected)
  # vm_exclude = [] # Inventory paths to exclude
  vm_metric_include = [
    "cpu.demand.average",
    "cpu.idle.summation",
    "cpu.latency.average",
    "cpu.readiness.average",
    "cpu.ready.summation",
    "cpu.run.summation",
    "cpu.usage.average",
    "cpu.used.summation",
    "cpu.wait.summation",
    "mem.active.average",
    "mem.granted.average",
    "mem.latency.average",
    "mem.swapin.average",
    "mem.swapinRate.average",
    "mem.swapout.average",
    "mem.swapoutRate.average",
    "mem.usage.average",
    "mem.vmmemctl.average",
    "net.bytesRx.average",
    "net.bytesTx.average",
    "net.droppedRx.summation",
    "net.droppedTx.summation",
    "net.usage.average",
    "power.power.average",
    "virtualDisk.numberReadAveraged.average",
    "virtualDisk.numberWriteAveraged.average",
    "virtualDisk.read.average",
    "virtualDisk.readOIO.latest",
    "virtualDisk.throughput.usage.average",
    "virtualDisk.totalReadLatency.average",
    "virtualDisk.totalWriteLatency.average",
    "virtualDisk.write.average",
    "virtualDisk.writeOIO.latest",
    "sys.uptime.latest",
  ]
  # vm_metric_exclude = [] ## Nothing is excluded by default
  # vm_instances = true ## true by default

  ## Hosts
  ## Typical host metrics (if omitted or empty, all metrics are collected)
  # host_include = [ "/*/host/**"] # Inventory path to hosts to collect (by default all are collected)
  # host_exclude [] # Inventory paths to exclude
  host_metric_include = [
    "cpu.coreUtilization.average",
    "cpu.costop.summation",
    "cpu.demand.average",
    "cpu.idle.summation",
    "cpu.latency.average",
    "cpu.readiness.average",
    "cpu.ready.summation",
    "cpu.swapwait.summation",
    "cpu.usage.average",
    "cpu.used.summation",
    "cpu.utilization.average",
    "cpu.wait.summation",
    "disk.deviceReadLatency.average",
    "disk.deviceWriteLatency.average",
    "disk.kernelReadLatency.average",
    "disk.kernelWriteLatency.average",
    "disk.numberReadAveraged.average",
    "disk.numberWriteAveraged.average",
    "disk.read.average",
    "disk.totalReadLatency.average",
    "disk.totalWriteLatency.average",
    "disk.write.average",
    "mem.active.average",
    "mem.latency.average",
    "mem.state.latest",
    "mem.swapin.average",
    "mem.swapinRate.average",
    "mem.swapout.average",
    "mem.swapoutRate.average",
    "mem.totalCapacity.average",
    "mem.usage.average",
    "mem.vmmemctl.average",
    "net.bytesRx.average",
    "net.bytesTx.average",
    "net.droppedRx.summation",
    "net.droppedTx.summation",
    "net.errorsRx.summation",
    "net.errorsTx.summation",
    "net.usage.average",
    "power.power.average",
    "storageAdapter.numberReadAveraged.average",
    "storageAdapter.numberWriteAveraged.average",
    "storageAdapter.read.average",
    "storageAdapter.write.average",
    "sys.uptime.latest",
  ]


  # host_instances = true ## true by default
  # host_include = [] ## Nothing included by default
  # host_exclude = [] ## Nothing excluded by default
  # host_metric_include = [] ## Nothing included by default
  # host_metric_exclude = [] ## Nothing excluded by default


  ## Clusters
  # cluster_include = [ "/*/host/**"] # Inventory path to clusters to collect (by default all are collected)
  # cluster_exclude = [] # Inventory paths to exclude
  # cluster_metric_include = [] ## if omitted or empty, all metrics are collected
  # cluster_metric_exclude = [] ## Nothing excluded by default
  # cluster_instances = false ## false by default

  ## Resource Pools
  # resoucepool_include = [ "/*/host/**"] # Inventory path to datastores to collect (by default all are collected)
  # resoucepool_exclude = [] # Inventory paths to exclude
  # resoucepool_metric_include = [] ## if omitted or empty, all metrics are collected
  # resoucepool_metric_exclude = [] ## Nothing excluded by default
  # resoucepool_instances = false ## false by default

  ## Datastores
  # datastore_include = [ "/*/datastore/**"] # Inventory path to datastores to collect (by default all are collected)
  # datastore_exclude = [] # Inventory paths to exclude
  # datastore_metric_include = [] ## if omitted or empty, all metrics are collected
  # datastore_metric_exclude = [] ## Nothing excluded by default
  # datastore_instances = false ## false by default

  ## Datacenters
  # datacenter_include = [ "/*/host/**"] # Inventory path to clusters to collect (by default all are collected)
  # datacenter_exclude = [] # Inventory paths to exclude
  # datacenter_metric_include = [] ## if omitted or empty, all metrics are collected
  # datacenter_metric_exclude = [ "*" ] ## Datacenters are not collected by default.
  # datacenter_instances = false ## false by default

  ## Plugin Settings
  ## separator character to use for measurement and field names (default: "_")
  # separator = "_"

   ## Collect IP addresses? Valid values are "ipv4" and "ipv6"
  # ip_addresses = ["ipv6", "ipv4" ]

  ## When set to true, all samples are sent as integers. This makes the output
  ## data types backwards compatible with Telegraf 1.9 or lower. Normally all
  ## samples from vCenter, with the exception of percentages, are integer
  ## values, but under some conditions, some averaging takes place internally in
  ## the plugin. Setting this flag to "false" will send values as floats to
  ## preserve the full precision when averaging takes place.
  # use_int_samples = true

  ## Custom attributes from vCenter can be very useful for queries in order to slice the
  ## metrics along different dimension and for forming ad-hoc relationships. They are disabled
  ## by default, since they can add a considerable amount of tags to the resulting metrics. To
  ## enable, simply set custom_attribute_exclude to [] (empty set) and use custom_attribute_include
  ## to select the attributes you want to include.
  ## By default, since they can add a considerable amount of tags to the resulting metrics. To
  ## enable, simply set custom_attribute_exclude to [] (empty set) and use custom_attribute_include
  ## to select the attributes you want to include.
  # custom_attribute_include = []
  # custom_attribute_exclude = ["*"]


  ## The number of vSphere 5 minute metric collection cycles to look back for non-realtime metrics. In
  ## some versions (6.7, 7.0 and possible more), certain metrics, such as cluster metrics, may be reported
  ## with a significant delay (>30min). If this happens, try increasing this number. Please note that increasing
  ## it too much may cause performance issues.
  # metric_lookback = 3

  ## number of objects to retrieve per query for realtime resources (vms and hosts)
  ## set to 64 for vCenter 5.5 and 6.0 (default: 256)
  # max_query_objects = 256

  ## number of metrics to retrieve per query for non-realtime resources (clusters and datastores)
  ## set to 64 for vCenter 5.5 and 6.0 (default: 256)
  # max_query_metrics = 256

  ## number of go routines to use for collection and discovery of objects and metrics
  # collect_concurrency = 1
  # discover_concurrency = 1

  ## the interval before (re)discovering objects subject to metrics collection (default: 300s)
  # object_discovery_interval = "300s"

  ## timeout applies to any of the api request made to vcenter
  # timeout = "60s"

  ## Optional SSL Config
  use_tls = true
  # tls_ca = "/path/to/cafile"
  # tls_cert = "/path/to/certfile"
  # tls_key = "/path/to/keyfile"
  ## Use SSL but skip chain & host verification
  insecure_skip_verify = true

  ## The Historical Interval value must match EXACTLY the interval in the daily
  # "Interval Duration" found on the VCenter server under Configure > General > Statistics > Statistic intervals
  # historical_interval = "5m"
```

### 仪表盘

[dashboard](../dashboards/vsphere.json)中选择变量，如果有特殊需求，可自行修改或者添加；

### 告警规则

[alerts](../alerts/alerts.json)
