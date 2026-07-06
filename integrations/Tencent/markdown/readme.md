# 腾讯云监控


## CVM 监控指标

### CPU 监控

| 指标英文名 | 指标中文名 | 指标说明 | 单位 | 维度 | 统计规则 [period, statType] |
| :--- | :--- | :--- | :--- | :--- | :--- |
| CpuUsage | CPU 利用率 | 机器运行期间实时占用的 CPU 百分比 | % | InstanceId | [10s, avg][60s, avg][300s, max][3600s, max][86400s, max] |
| CpuLoadavg | CPU 一分钟平均负载 | 1分钟内正在使用和等待使用 CPU 的平均任务数（Windows 机器无此指标） | - | InstanceId | [10s, avg] [60s, avg] [300s, max] [3600s, max] [86400s, max] |
| Cpuloadavg5m | CPU 五分钟平均负载 | 5分钟内正在使用和等待使用 CPU 的平均任务数（Windows 机器无此指标） | 0 | InstanceId | [60s, avg] [300s, max] |
| Cpuloadavg15m | CPU 十五分钟平均负载 | 15分钟内正在使用和等待使用 CPU 的平均任务数（Windows 机器无此指标） | 0 | InstanceId | [60s, avg] [300s, max] |
| BaseCpuUsage | 基础 CPU 使用率 | 基础 CPU 使用率通过宿主机采集上报，无须安装监控组件即可查看数据，子机高负载情况下仍可持续采集上报数据 | % | InstanceId | [10s, avg][60s, avg][300s, max][3600s, max, avg][86400s, max] |

### GPU 监控

| 指标英文名 | 指标中文名 | 指标说明 | 单位 | 维度 | 统计规则 [period, statType] |
| :--- | :--- | :--- | :--- | :--- | :--- |
| GpuMemTotal | GPU 内存总量 | GPU 内存总量 | MB | InstanceId | [10s, avg] [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |
| Gpumemusage | GPU 内存使用率 | GPU 内存使用率 | % | InstanceId | [10s, avg] [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |
| GpuMemUsed | GPU 内存使用量 | 评估负载对显存的占用 | MB | InstanceId | [10s, avg] [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |
| Gpupowdraw | GPU 功耗使用量 | GPU 功耗使用量 | 0 | InstanceId | [10s, avg] [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |
| Gpupowlimit | GPU 功耗总量 | GPU 功耗总量 | 0 | InstanceId | [10s, avg] [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |
| Gpupowusage | GPU 功耗使用率 | GPU 功耗使用率 | % | InstanceId | [10s, avg] [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |
| Gputemp | GPU 温度 | 评估 GPU 散热状态 | 0 | InstanceId | [10s, avg] [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |
| Gpuutil | GPU 使用率 | 评估负载所消耗的计算能力，非空闲状态百分比 | % | InstanceId | [10s, avg] [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |
| GpuEncUtil | GPU 编码器使用率 | GPU 编码器使用率 | % | InstanceId | [10s, avg] [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |
| GpuDecUtil | GPU 解码器使用率 | GPU 解码器使用率 | % | InstanceId | [10s, avg] [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |

### 网络监控

| 指标英文名 | 指标中文名 | 指标说明 | 单位 | 维度 | 统计规则 [period, statType] |
| :--- | :--- | :--- | :--- | :--- | :--- |
| LanOuttraffic | 内网出带宽 | 内网网卡的平均每秒出流量 | Mbps | InstanceId | [10s, avg] [60s, avg] [300s, max] [3600s, max] [86400s, max] |
| LanIntraffic | 内网入带宽 | 内网网卡的平均每秒入流量 | Mbps | InstanceId | [10s, avg] [60s, avg] [300s, max] [3600s, max] [86400s, max] |
| WanOuttraffic | 外网出带宽 | 外网网卡的平均每秒出流量 | Mbps | InstanceId | [10s, avg] [60s, avg] [300s, max] [3600s, max] [86400s, max] |
| WanIntraffic | 外网入带宽 | 外网网卡的平均每秒入流量 | Mbps | InstanceId | [10s, avg] [60s, avg] [300s, max] [3600s, max] [86400s, max] |
| AccOuttraffic | 外网出流量 | 外网网卡出流量累计值 | MB | InstanceId | [60s, sum] [300s, sum] [3600s, sum] [86400s, sum] |
| AccIntraffic | 外网入流量 | 外网网卡入流量累计值 | MB | InstanceId | [60s, sum] [300s, sum] [3600s, sum] [86400s, sum] |
| PackDrop | 丢包率 | 内网和外网的平均每秒丢包数 | pps | InstanceId | [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |
| OutPkg | 出包量 | 内网和外网的平均每秒出包数 | pps | InstanceId | [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |
| InPkg | 入包量 | 内网和外网的平均每秒入包数 | pps | InstanceId | [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |

### 内存监控

| 指标英文名 | 指标中文名 | 指标说明 | 单位 | 维度 | 统计规则 [period, statType] |
| :--- | :--- | :--- | :--- | :--- | :--- |
| MemUsage | 内存利用率 | 用户实际使用的内存量，不包括系统缓存和缓冲占用的内存 | % | InstanceId | [10s, avg] [60s, avg] [300s, max] [3600s, max] [86400s, max] |
| MemUsed | 内存使用量 | 用户实际使用的内存量，不包括系统缓存和缓冲占用的内存 | MB | InstanceId | [10s, avg] [60s, avg] [300s, max] [3600s, max] [86400s, max] |
| MemTotal | 内存总量 | 用户购买云服务器时配置的内存大小 | MB | InstanceId | [10s, avg] [60s, avg] [300s, max] [3600s, max] [86400s, max] |
| MemAvailable | 可用内存 | 可用内存量，包括系统缓存和缓冲 | MB | InstanceId | [10s, avg] [60s, avg] [300s, max] [3600s, max] [86400s, max] |


## TDSQL MySQL 监控指标

| 指标英文名 | 指标中文名 | 说明 | 单位 | 维度 | 统计规则<br>[period, statType] |
| :--- | :--- | :--- | :--- | :--- | :--- |
| BytesReceived | 每秒接收客户端流量 | 每秒接收客户端流量 | MB/s | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| BytesSent | 每秒发送客户端流量 | 每秒发送客户端流量 | MB/s | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Ccu | CCU_仅对于 serverless | CCU_仅对于 serverless | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Comcommit | 提交数 | 提交数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| ComDelete | 删除数 | 删除数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| ComInsert | 插入数 | 插入数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Comreplace | 覆盖数 | 覆盖数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Comrollback | 回滚数 | 回滚数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| ComSelect | 查询数 | 查询数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| ComUpdate | 更新数 | 更新数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Connectionuserate | 连接数利用率 | 连接数利用率 | % | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Cpuuserate | CPU 使用率 | CPU 使用率 | % | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Createdtmpdisktables | 临时表数量 | 临时表数量 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Createdtmpfiles | 临时文件数量 | 临时文件数量 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| CreatedTmpTables | 临时表的数量 | 临时表的数量 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| DataVolumeUsage | 数据表空间使用量 | 数据表空间使用量 | GB | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| HandlerCommit | 内部提交数 | 内部提交数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Handlerreadrndnext | 读下一行请求数 | 读下一行请求数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| HandlerRollback | 内部回滚数 | 内部回滚数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Innodbbufferpoolpagesdirty | InnoDB 脏页数 | InnoDB 脏页数 | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Innodbbufferpoolpagesfree | InnoDB 空页数 | InnoDB 空页数 | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Innodbbufferpoolpagestotal | InnoDB 总页数 | InnoDB 总页数 | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| InnodbBufferPoolReadRequests | InnoDB 缓冲池读取次数 | InnoDB 缓冲池读取次数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Innodbbufferpoolreads | InnoDB 物理读 | InnoDB 物理读 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Innodbbufferpoolwriterequest | InnoDB 缓冲池写入次数 | InnoDB 缓冲池写入次数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| InnodbBufferPoolWriteRequests | InnoDB 引擎每秒已完成的逻辑写请求次数 | InnoDB 引擎每秒已完成的逻辑写请求次数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| InnodbCacheHitRate | InnoDB 引擎缓存命中率 | InnoDB 引擎缓存命中率 | % | InstanceId | [ 5s, min ]<br>[ 60s, min ]<br>[ 300s, min ]<br>[ 3600s, min ]<br>[ 86400s, min ] |
| InnodbCacheUseRate | InnoDB 引擎缓存使用率 | InnoDB 引擎缓存使用率 | % | InstanceId | [ 5s, min ]<br>[ 60s, min ]<br>[ 300s, min ]<br>[ 3600s, min ]<br>[ 86400s, min ] |
| Innodbdatapendingreads | InnoDB 挂起读取数 | InnoDB 挂起读取数 | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Innodbdatapendingwrites | InnoDB挂起写入数 | InnoDB挂起写入数 | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Innodbdataread | InnoDB 读取量 | InnoDB 读取量 | Bytes | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Innodbdatareads | InnoDB 总读取量 | InnoDB 总读取量 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Innodbdatawrites | InnoDB 总写入量 | InnoDB 总写入量 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Innodbdatawritten | InnoDB 写入量 | InnoDB 写入量 | Bytes | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Innodblogwaits | InnoDB 日志等待写入次数 | InnoDB 日志等待写入次数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Innodblogwriterequests | InnoDB 日志物理写请求次数 | InnoDB 日志物理写请求次数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Innodblogwrites | InnoDB 日志物理写入次数 | InnoDB 日志物理写入次数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Innodbnumopenfiles | 当前 InnoDB 打开表的数量 | 当前 InnoDB 打开表的数量 | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Innodbosfilereads | 读磁盘数量 | 读磁盘数量 | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Innodbosfilewrites | 写磁盘数量 | 写磁盘数量 | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Innodbosfsyncs | InnoDB_fsyncs 数 | InnoDB_fsyncs 数 | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Innodbrowlocktimeavg | InnoDB 平均获取行锁时间 | InnoDB 平均获取行锁时间 | ms | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Innodbrowlockwaits | InnoDB 等待行锁次数 | InnoDB 等待行锁次数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| InnodbRowsDeleted | InnoDB 行删除量 | InnoDB 行删除量 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| InnodbRowsInserted | InnoDB 行插入量 | InnoDB 行插入量 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| InnodbRowsRead | InnoDB 行读取量 | InnoDB 行读取量 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| InnodbRowsUpdated | InnoDB 行更新量 | InnoDB 行更新量 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| MaxConnections | 最大连接数 | 最大连接数 | count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| MemoryUse | 内存使用量 | 内存使用量 | MB | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Memoryuserate | 内存使用率 | 内存使用率 | % | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Openedtables | 已经打开的表数 | 已经打开的表数 | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Openfiles | 打开文件总数 | 打开文件总数 | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Proxycpuuserate | CPU 利用率 | CPU 利用率 | % | ProxyNodeId | [ 5s, max ]<br>[ 10s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Proxycurrentconnections | 当前连接数 | 当前连接数 | Count/s | ProxyNodeId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Proxymemoryusage | 内存占用 | 内存占用 | MBytes | ProxyNodeId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Proxymemoryuserate | 内存利用率 | 内存利用率 | % | ProxyNodeId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Proxyqueries | 请求数 | 请求数 | Count/s | ProxyNodeId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Proxyroutemaster | 写请求数 | 写请求数 | Count/s | ProxyNodeId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Proxyrouteslave | 读请求数 | 读请求数 | Count/s | ProxyNodeId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Qcachehitrate | Qcache 命中率 | Qcache 命中率 | % | InstanceId | [ 5s, min ]<br>[ 60s, min ]<br>[ 300s, min ]<br>[ 3600s, min ]<br>[ 86400s, min ] |
| Qcacheuserate | Qcache使用率 | Qcache使用率 | % | InstanceId | [ 5s, min ]<br>[ 60s, min ]<br>[ 300s, min ]<br>[ 3600s, min ]<br>[ 86400s, min ] |
| Qps | 每秒执行操作数 | 每秒执行操作数 | Count/s | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Queries | 总请求数 | 总请求数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Readiops | 读请求 IOPS | 读请求 IOPS | Count/s | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Replicationdelay | 复制延迟 | 复制延迟 | ms | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Replicationdelaydistance | 复制落后的 lsn 距离 | 复制落后的 lsn 距离 | Bytes | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Replicationstatus | 复制状态 | 复制状态 | None | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Selectfulljoin | 全表扫描复合查询次数 | 全表扫描复合查询次数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Selectfullrangejoin | 范围扫描复合查询次数 | 范围扫描复合查询次数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Selectscan | 全表扫描数 | 全表扫描数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| SlowQueries | 慢查询数 | 慢查询数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Sortmergepasses | 排序合并通过次数 | 排序合并通过次数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Storageuse | 存储使用量 | 存储使用量 | GBytes | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Storageuserate | 存储使用率 | 存储使用率 | % | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Tablelocksimmediate | 立即释放的表锁数 | 立即释放的表锁数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Tablelockswaited | 等待表锁次数 | 等待表锁次数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Tableopencachehits | 表打开缓存命中数 | 表打开缓存命中数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Tableopencachemisses | 表打开缓存未命中数 | 表打开缓存未命中数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Threadsconnected | 当前打开连接数 | 当前打开连接数 | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Threadscreated | 已创建的线程数 | 已创建的线程数 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| ThreadsRunning | 运行的线程数 | 运行的线程数 | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| TmpVolumeUsage | 临时表空间使用量 | 临时表空间使用量 | GB | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Tps | 每秒执行事务数 | 每秒执行事务数 | Count/s | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Txsqlparallelstmterror | 并行查询错误数 | 并行查询报错的语句数量 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Txsqlparallelstmtexecuted | 已执行并行查询数 | 已执行的并行查询语句数量 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Txsqlparallelstmtfallback | 回滚串行查询数 | 并行查询回滚到串行查询的语句数量 | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Txsqlparallelthreadscurrentlyused | 当前并行查询线程数 | 并行查询当前使用的线程数量 | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| UndoVolumeUsage | undo 表空间使用量 | undo 表空间使用量 | GB | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Writeiops | 写请求 IOPS | 写请求 IOPS | Count/s | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |