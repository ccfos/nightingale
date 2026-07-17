# Tencent Cloud Monitoring


## CVM Monitoring Metrics

### CPU Monitoring

| Metric Name | Display Name | Description | Unit | Dimensions | Statistics Rules [period, statType] |
| :--- | :--- | :--- | :--- | :--- | :--- |
| CpuUsage | CPU Utilization | Real-time percentage of CPU used while the machine is running | % | InstanceId | [10s, avg][60s, avg][300s, max][3600s, max][86400s, max] |
| CpuLoadavg | 1-Minute Average CPU Load | Average number of tasks using or waiting for the CPU in the past 1 minute (not available on Windows machines) | - | InstanceId | [10s, avg] [60s, avg] [300s, max] [3600s, max] [86400s, max] |
| Cpuloadavg5m | 5-Minute Average CPU Load | Average number of tasks using or waiting for the CPU in the past 5 minutes (not available on Windows machines) | 0 | InstanceId | [60s, avg] [300s, max] |
| Cpuloadavg15m | 15-Minute Average CPU Load | Average number of tasks using or waiting for the CPU in the past 15 minutes (not available on Windows machines) | 0 | InstanceId | [60s, avg] [300s, max] |
| BaseCpuUsage | Base CPU Utilization | Base CPU utilization is collected and reported by the host machine, so data is available without installing any monitoring agent, and collection continues even when the guest instance is under high load | % | InstanceId | [10s, avg][60s, avg][300s, max][3600s, max, avg][86400s, max] |

### GPU Monitoring

| Metric Name | Display Name | Description | Unit | Dimensions | Statistics Rules [period, statType] |
| :--- | :--- | :--- | :--- | :--- | :--- |
| GpuMemTotal | Total GPU Memory | Total GPU memory | MB | InstanceId | [10s, avg] [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |
| Gpumemusage | GPU Memory Utilization | GPU memory utilization | % | InstanceId | [10s, avg] [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |
| GpuMemUsed | GPU Memory Used | Measures how much GPU memory the workload occupies | MB | InstanceId | [10s, avg] [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |
| Gpupowdraw | GPU Power Draw | GPU power draw | 0 | InstanceId | [10s, avg] [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |
| Gpupowlimit | Total GPU Power | Total GPU power | 0 | InstanceId | [10s, avg] [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |
| Gpupowusage | GPU Power Utilization | GPU power utilization | % | InstanceId | [10s, avg] [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |
| Gputemp | GPU Temperature | Measures the GPU thermal status | 0 | InstanceId | [10s, avg] [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |
| Gpuutil | GPU Utilization | Measures the compute capacity consumed by the workload, as the percentage of non-idle time | % | InstanceId | [10s, avg] [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |
| GpuEncUtil | GPU Encoder Utilization | GPU encoder utilization | % | InstanceId | [10s, avg] [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |
| GpuDecUtil | GPU Decoder Utilization | GPU decoder utilization | % | InstanceId | [10s, avg] [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |

### Network Monitoring

| Metric Name | Display Name | Description | Unit | Dimensions | Statistics Rules [period, statType] |
| :--- | :--- | :--- | :--- | :--- | :--- |
| LanOuttraffic | Private Network Outbound Bandwidth | Average outbound traffic per second on the private network interface | Mbps | InstanceId | [10s, avg] [60s, avg] [300s, max] [3600s, max] [86400s, max] |
| LanIntraffic | Private Network Inbound Bandwidth | Average inbound traffic per second on the private network interface | Mbps | InstanceId | [10s, avg] [60s, avg] [300s, max] [3600s, max] [86400s, max] |
| WanOuttraffic | Public Network Outbound Bandwidth | Average outbound traffic per second on the public network interface | Mbps | InstanceId | [10s, avg] [60s, avg] [300s, max] [3600s, max] [86400s, max] |
| WanIntraffic | Public Network Inbound Bandwidth | Average inbound traffic per second on the public network interface | Mbps | InstanceId | [10s, avg] [60s, avg] [300s, max] [3600s, max] [86400s, max] |
| AccOuttraffic | Public Network Outbound Traffic | Cumulative outbound traffic on the public network interface | MB | InstanceId | [60s, sum] [300s, sum] [3600s, sum] [86400s, sum] |
| AccIntraffic | Public Network Inbound Traffic | Cumulative inbound traffic on the public network interface | MB | InstanceId | [60s, sum] [300s, sum] [3600s, sum] [86400s, sum] |
| PackDrop | Packet Loss Rate | Average number of packets dropped per second on the private and public networks | pps | InstanceId | [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |
| OutPkg | Outbound Packets | Average number of packets sent per second on the private and public networks | pps | InstanceId | [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |
| InPkg | Inbound Packets | Average number of packets received per second on the private and public networks | pps | InstanceId | [60s, avg] [300s, avg] [3600s, avg] [86400s, avg] |

### Memory Monitoring

| Metric Name | Display Name | Description | Unit | Dimensions | Statistics Rules [period, statType] |
| :--- | :--- | :--- | :--- | :--- | :--- |
| MemUsage | Memory Utilization | Amount of memory actually used by the user, excluding memory used by system caches and buffers | % | InstanceId | [10s, avg] [60s, avg] [300s, max] [3600s, max] [86400s, max] |
| MemUsed | Memory Used | Amount of memory actually used by the user, excluding memory used by system caches and buffers | MB | InstanceId | [10s, avg] [60s, avg] [300s, max] [3600s, max] [86400s, max] |
| MemTotal | Total Memory | Memory size configured when the user purchased the cloud server | MB | InstanceId | [10s, avg] [60s, avg] [300s, max] [3600s, max] [86400s, max] |
| MemAvailable | Available Memory | Amount of available memory, including system caches and buffers | MB | InstanceId | [10s, avg] [60s, avg] [300s, max] [3600s, max] [86400s, max] |


## TDSQL MySQL Monitoring Metrics

| Metric Name | Display Name | Description | Unit | Dimensions | Statistics Rules<br>[period, statType] |
| :--- | :--- | :--- | :--- | :--- | :--- |
| BytesReceived | Traffic received from clients per second | Traffic received from clients per second | MB/s | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| BytesSent | Traffic sent to clients per second | Traffic sent to clients per second | MB/s | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Ccu | CCU (serverless only) | CCU (serverless only) | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Comcommit | Commits | Number of commits | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| ComDelete | Deletes | Number of deletes | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| ComInsert | Inserts | Number of inserts | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Comreplace | Replaces | Number of replaces | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Comrollback | Rollbacks | Number of rollbacks | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| ComSelect | Selects | Number of select queries | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| ComUpdate | Updates | Number of updates | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Connectionuserate | Connection utilization | Connection utilization | % | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Cpuuserate | CPU utilization | CPU utilization | % | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Createdtmpdisktables | Temporary disk tables | Number of temporary disk tables | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Createdtmpfiles | Temporary files | Number of temporary files | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| CreatedTmpTables | Temporary tables | Number of temporary tables | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| DataVolumeUsage | Data tablespace usage | Data tablespace usage | GB | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| HandlerCommit | Internal commits | Number of internal commits | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Handlerreadrndnext | Read-next-row requests | Number of read-next-row requests | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| HandlerRollback | Internal rollbacks | Number of internal rollbacks | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Innodbbufferpoolpagesdirty | InnoDB dirty pages | Number of InnoDB dirty pages | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Innodbbufferpoolpagesfree | InnoDB free pages | Number of InnoDB free pages | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Innodbbufferpoolpagestotal | InnoDB total pages | Total number of InnoDB pages | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| InnodbBufferPoolReadRequests | InnoDB buffer pool read requests | Number of InnoDB buffer pool read requests | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Innodbbufferpoolreads | InnoDB physical reads | Number of InnoDB physical reads | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Innodbbufferpoolwriterequest | InnoDB buffer pool write requests | Number of InnoDB buffer pool write requests | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| InnodbBufferPoolWriteRequests | Logical write requests completed per second by the InnoDB engine | Logical write requests completed per second by the InnoDB engine | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| InnodbCacheHitRate | InnoDB engine cache hit rate | InnoDB engine cache hit rate | % | InstanceId | [ 5s, min ]<br>[ 60s, min ]<br>[ 300s, min ]<br>[ 3600s, min ]<br>[ 86400s, min ] |
| InnodbCacheUseRate | InnoDB engine cache utilization | InnoDB engine cache utilization | % | InstanceId | [ 5s, min ]<br>[ 60s, min ]<br>[ 300s, min ]<br>[ 3600s, min ]<br>[ 86400s, min ] |
| Innodbdatapendingreads | InnoDB pending reads | Number of InnoDB pending reads | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Innodbdatapendingwrites | InnoDB pending writes | Number of InnoDB pending writes | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Innodbdataread | InnoDB data read | Amount of InnoDB data read | Bytes | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Innodbdatareads | InnoDB total reads | Total number of InnoDB reads | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Innodbdatawrites | InnoDB total writes | Total number of InnoDB writes | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Innodbdatawritten | InnoDB data written | Amount of InnoDB data written | Bytes | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Innodblogwaits | InnoDB log write waits | Number of times InnoDB log waited for writes | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Innodblogwriterequests | InnoDB log physical write requests | Number of InnoDB log physical write requests | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Innodblogwrites | InnoDB log physical writes | Number of InnoDB log physical writes | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Innodbnumopenfiles | Tables currently open in InnoDB | Number of tables currently open in InnoDB | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Innodbosfilereads | Disk reads | Number of disk reads | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Innodbosfilewrites | Disk writes | Number of disk writes | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Innodbosfsyncs | InnoDB fsyncs | Number of InnoDB fsyncs | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Innodbrowlocktimeavg | Average InnoDB row lock acquisition time | Average InnoDB row lock acquisition time | ms | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Innodbrowlockwaits | InnoDB row lock waits | Number of InnoDB row lock waits | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| InnodbRowsDeleted | InnoDB rows deleted | Number of InnoDB rows deleted | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| InnodbRowsInserted | InnoDB rows inserted | Number of InnoDB rows inserted | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| InnodbRowsRead | InnoDB rows read | Number of InnoDB rows read | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| InnodbRowsUpdated | InnoDB rows updated | Number of InnoDB rows updated | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| MaxConnections | Maximum connections | Maximum number of connections | count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| MemoryUse | Memory usage | Memory usage | MB | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Memoryuserate | Memory utilization | Memory utilization | % | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Openedtables | Opened tables | Number of tables that have been opened | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Openfiles | Total open files | Total number of open files | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Proxycpuuserate | CPU utilization | CPU utilization | % | ProxyNodeId | [ 5s, max ]<br>[ 10s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Proxycurrentconnections | Current connections | Number of current connections | Count/s | ProxyNodeId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Proxymemoryusage | Memory usage | Memory usage | MBytes | ProxyNodeId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Proxymemoryuserate | Memory utilization | Memory utilization | % | ProxyNodeId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Proxyqueries | Requests | Number of requests | Count/s | ProxyNodeId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Proxyroutemaster | Write requests | Number of write requests | Count/s | ProxyNodeId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Proxyrouteslave | Read requests | Number of read requests | Count/s | ProxyNodeId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Qcachehitrate | Qcache hit rate | Qcache hit rate | % | InstanceId | [ 5s, min ]<br>[ 60s, min ]<br>[ 300s, min ]<br>[ 3600s, min ]<br>[ 86400s, min ] |
| Qcacheuserate | Qcache utilization | Qcache utilization | % | InstanceId | [ 5s, min ]<br>[ 60s, min ]<br>[ 300s, min ]<br>[ 3600s, min ]<br>[ 86400s, min ] |
| Qps | Operations per second | Number of operations executed per second | Count/s | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Queries | Total requests | Total number of requests | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Readiops | Read IOPS | Read request IOPS | Count/s | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Replicationdelay | Replication delay | Replication delay | ms | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Replicationdelaydistance | Replication lag LSN distance | LSN distance the replica lags behind | Bytes | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Replicationstatus | Replication status | Replication status | None | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Selectfulljoin | Full-table-scan join queries | Number of join queries using full table scans | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Selectfullrangejoin | Range-scan join queries | Number of join queries using range scans | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Selectscan | Full table scans | Number of full table scans | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| SlowQueries | Slow queries | Number of slow queries | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Sortmergepasses | Sort merge passes | Number of sort merge passes | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Storageuse | Storage usage | Storage usage | GBytes | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Storageuserate | Storage utilization | Storage utilization | % | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Tablelocksimmediate | Table locks released immediately | Number of table locks released immediately | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Tablelockswaited | Table lock waits | Number of table lock waits | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Tableopencachehits | Table open cache hits | Number of table open cache hits | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Tableopencachemisses | Table open cache misses | Number of table open cache misses | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Threadsconnected | Currently open connections | Number of currently open connections | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Threadscreated | Threads created | Number of threads created | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| ThreadsRunning | Threads running | Number of running threads | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| TmpVolumeUsage | Temporary tablespace usage | Temporary tablespace usage | GB | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Tps | Transactions per second | Number of transactions executed per second | Count/s | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Txsqlparallelstmterror | Parallel query errors | Number of parallel query statements that reported errors | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Txsqlparallelstmtexecuted | Parallel queries executed | Number of parallel query statements executed | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Txsqlparallelstmtfallback | Parallel queries fell back to serial | Number of parallel query statements that fell back to serial execution | Count | InstanceId | [ 5s, sum ]<br>[ 60s, sum ]<br>[ 300s, sum ]<br>[ 3600s, sum ]<br>[ 86400s, sum ] |
| Txsqlparallelthreadscurrentlyused | Current parallel query threads | Number of threads currently used by parallel queries | Count | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| UndoVolumeUsage | Undo tablespace usage | Undo tablespace usage | GB | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
| Writeiops | Write IOPS | Write request IOPS | Count/s | InstanceId | [ 5s, max ]<br>[ 60s, max ]<br>[ 300s, max ]<br>[ 3600s, max ]<br>[ 86400s, max ] |
