# netstat_filter

该插件采集网络连接情况，并根据用户条件进行过滤统计，以达到监控用户关心链接情况
## 指标列表
tcp_established  
tcp_syn_sent
tcp_syn_recv
tcp_fin_wait1
tcp_fin_wait2
tcp_time_wait
tcp_close
tcp_close_wait
tcp_last_ack
tcp_listen
tcp_closing
tcp_none
tcp_send_queue
tcp_recv_queue

## 功能说明
对源IP、源端口、目标IP和目标端口过滤后进行网卡recv-Q、send-Q进行采集，该指标可以很好反应出指定连接的质量，例如rtt时间过长，导致收到服务端ack确认很慢就会使send-Q长期大于0，可以及时通过监控发现，从而提前优化网络或程序

当过滤结果为多个连接时会将send和recv值进行加和
例如：
配置文件``raddr_port = 11883``
当本地和不同IP的11883都有连接建立的情况下，会将多条连接的结果进行加和。或在并发多连接的情况下，会合并加合，总之过滤的越粗略被加合数就会越多。

多条规则请复制``[[instances]]``进行配置

## 注意事项
netstat_filter_tcp_send_queue和netstat_filter_tcp_recv_queue指标目前只支持linux。windows用户默认为0。
