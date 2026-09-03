# nsq
forked from [telegraf/nsq](https://github.com/influxdata/telegraf/blob/master/plugins/inputs/nsq/nsq.go)
## Configuration
- Configuration file: see the [reference example](https://github.com/flashcatcloud/categraf/blob/main/conf/input.nsq/nsq.toml)

## Metric List
### nsq_client metrics
ready_count     number of messages ready to be consumed
inflight_count  number of messages currently being processed
message_count   total number of messages
finish_count    number of finished messages
requeue_count   number of requeued messages

### nsq_channel metrics
depth    current backlog size
backend_depth   backlog size of the message buffer queue
inflight_count  number of messages currently being processed
deferred_count  number of deferred messages
message_count   total number of messages
requeue_count   number of requeued messages
timeout_count   number of timed-out messages
client_count    number of clients

### nsq_topic metrics
depth    message queue backlog size
backend_depth  backlog size of the message buffer queue
message_count   total number of messages
channel_count   total number of consumers

## metrics
The following can be cloned into Nightingale's metrics.yaml file as metric descriptions
# [nsq]
nsq_server_server_count: "total number of nsq servers"
nsq_server_topic_count: "total number of nsq topics"

nsq_topic_depth: message queue backlog size
nsq_topic_backend_depth: backlog size of the message buffer queue
nsq_topic_message_count: total number of messages
nsq_topic_channel_count: total number of consumers

nsq_channel_depth: "current number of messages, including those in memory and spilled to disk, i.e. the current backlog"
nsq_channel_backend_depth: backlog size of the message buffer queue
nsq_channel_inflight_count: "number of currently unfinished messages: the sum of messages sent but not yet FINed, requeued (REQ), and timed out (TIMEOUT); i.e. messages delivered but not yet consumed"
nsq_channel_deferred_count: "number of requeued deferred messages, i.e. requeued messages not yet republished; in other words, unconsumed scheduled (delayed) messages"
nsq_channel_message_count: total number of new messages since the node started, the true message count
nsq_channel_requeue_count: number of requeued messages, i.e. messages that returned REQ
nsq_channel_timeout_count: number of messages that were requeued but received a response within the configured timeout
nsq_channel_client_count: number of client connections

nsq_client_ready_count: number of messages the client is ready to consume
nsq_client_inflight_count: number of messages the client is currently processing
nsq_client_message_count: total number of client messages
nsq_client_finish_count: number of messages the client finished, i.e. messages that returned FIN
nsq_client_requeue_count: number of messages the client requeued, i.e. messages that returned REQ
