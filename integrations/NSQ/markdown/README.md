# nsq
forked from [telegraf/nsq](https://github.com/influxdata/telegraf/blob/master/plugins/inputs/nsq/nsq.go)
## Configuration
- 配置文件，[参考示例](https://github.com/flashcatcloud/categraf/blob/main/conf/input.nsq/nsq.toml)

## 指标列表
### nsq_client类
ready_count     可消费消息数
inflight_count  正在处理消息数
message_count   消息总数
finish_count    完成统计
requeue_count   重新排队消息数

### nsq_channel类
depth    当前的积压量
backend_depth   消息缓冲队列积压量
inflight_count  正在处理消息数
deferred_count  延迟消息数
message_count   消息总数
requeue_count   重新排队消息数
timeout_count   超时消息数
client_count    客户端数量

### nsq_topic类
depth    消息队列积压量
backend_depth  消息缓冲队列积压量
message_count   消息总数
channel_count   消费者总数

## metrics
此配置可 克隆到nightingale的metrics.yaml文件中作为中文指标解释
# [nsq]
nsq_server_server_count: "nsq 服务端总计"
nsq_server_topic_count: "nsq topic总数"

nsq_topic_depth: 消息队列积压量
nsq_topic_backend_depth: 消息缓冲队列积压量
nsq_topic_message_count: 消息总数
nsq_topic_channel_count: 消费者总数

nsq_channel_depth: "当前消息数,内存和硬盘转存的消息数，即当前的积压量"
nsq_channel_backend_depth: 消息缓冲队列积压量
nsq_channel_inflight_count: "当前未完成的消息数,包括发送但未返回FIN/重新入队列REQ/超时TIMEOUT 三种消息数之和，代表已经投递还未消费掉的消息"
nsq_channel_deferred_count: "重新入队的延迟消息数，指还未发布的重入队消息数量，即未消费的定时（延时）消息数"
nsq_channel_message_count: 节点启动后的所有新消息总数，真正的消息次数
nsq_channel_requeue_count: 重新入队的消息数，即返回REQ的消息数量
nsq_channel_timeout_count: 已重入队列但按配置的超时时间内还收到响应的消息数
nsq_channel_client_count: 客户端连接数

nsq_client_ready_count: 客户端可消费消息数
nsq_client_inflight_count: 客户端正在处理消息数
nsq_client_message_count: 客户端消息总数
nsq_client_finish_count: 客户端完成的消息数，即返回FIN的消息数
nsq_client_requeue_count: 客户端重新入队的消息数，即返回REQ的消息数量
