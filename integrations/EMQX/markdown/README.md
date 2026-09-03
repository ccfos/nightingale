emqx插件通过emqx的dashboard api来获取指标.

1. 配置及说明

```toml
# # 采集周期, 这里指定的interval会覆盖config.toml中的interval
# interval = 60
[[instances]]
## emqx的dashboard地址，ip:port, 单个集群监控使用逗号分割多个IP,会随机挑一个可用ip进行连接请求
## 多个集群，拆分到多个instance配置
addrees="http://192.168.11.11:18083,http://192.168.11.12:18083"
## 指定emqx的版本，支持v3/v4/v5, 分别对应emqx的3.X/4.X/5.X 版本
version="v3"
## api需要鉴权，输入用户名和密码
username="admin"
password="public"
```

2. 插件会先调用`/api/${version}/nodes` 获取node的版本、运行状态、连接数等信息, 接口返回
```json
{
  "code": 0,
  "data": [
    {
      "connections": 0,
      "load1": "0.33",
      "load15": "0.37",
      "load5": "0.36",
      "max_fds": 1048576,
      "memory_total": "191.77M",
      "memory_used": "129.38M",
      "name": "emqx@node2.emqx.io",
      "node": "emqx@node2.emqx.io",
      "node_status": "Running",
      "otp_release": "R21/10.3.5.6",
      "process_available": 2097152,
      "process_used": 509,
      "uptime": "3 hours, 12 minutes, 25 seconds",
      "version": "3.2.6"
    },
    {
      "connections": 0,
      "load1": "0.33",
      "load15": "0.37",
      "load5": "0.36",
      "max_fds": 1048576,
      "memory_total": "190.37M",
      "memory_used": "129.82M",
      "name": "emqx@node1.emqx.io",
      "node": "emqx@node1.emqx.io",
      "node_status": "Running",
      "otp_release": "R21/10.3.5.6",
      "process_available": 2097152,
      "process_used": 508,
      "uptime": "3 hours, 12 minutes, 45 seconds",
      "version": "3.2.6"
    }
  ]
}
```
对应生成的指标如下
```
emqx_node_connections{node="xxx"}  10
emqx_node_status{node="xxxx"} 1 # 1表示running
emqx_node_info{node="xxxx", name="xxxx", version="3.2.6", otp_release="R21/10.3.5.6"} 1
emqx_node_load1{node="xxxx"} 0.10
emqx_node_load5{node="xxxx"} 0.37
emqx_node_load15{node="xxxx"} 0.32
emqx_node_max_fds{node="xxxx"} 1048576
emqx_node_memory_total_bytes{node="xxxx"} 194938
emqx_node_memory_used_bytes{node="xxxx"} 132956
emqx_node_process_available{node="xxxx"} 2097152
emqx_node_process_used{node="xxxx"} 508
emqx_node_uptime_seconds{node="xxxx"} 1234567 #单位秒
```
会额外生成两个指标, 分别表示当前集群中有多少个node正常运行和已经停止了,标签中的cluster就是配置中的address
```
emqx_cluster_node_running{cluster="xxx"} 2
emqx_cluster_node_stopped{cluster="xxx"} 0
```

调用 `/api/${version}/stats`接口，获取每个node的统计信息,接口返回
```json
{
  "code": 0,
  "data": [
    {
      "node": "emqx@node2.emqx.io",
      "subscriptions.shared.max": 0,
      "subscriptions.max": 0,
      "subscribers.max": 0,
      "resources.max": 0,
      "topics.count": 0,
      "subscriptions.count": 0,
      "suboptions.max": 0,
      "topics.max": 0,
      "sessions.persistent.max": 0,
      "connections.max": 0,
      "sessions.persistent.count": 0,
      "actions.count": 5,
      "retained.count": 5,
      "rules.count": 0,
      "routes.count": 0,
      "subscriptions.shared.count": 0,
      "suboptions.count": 0,
      "sessions.count": 0,
      "actions.max": 5,
      "retained.max": 5,
      "sessions.max": 0,
      "rules.max": 0,
      "routes.max": 0,
      "resources.count": 0,
      "subscribers.count": 0,
      "connections.count": 0
    },
    {
      "node": "emqx@node1.emqx.io",
      "subscriptions.shared.max": 0,
      "subscriptions.max": 0,
      "subscribers.max": 0,
      "resources.max": 0,
      "topics.count": 0,
      "subscriptions.count": 0,
      "suboptions.max": 0,
      "topics.max": 0,
      "sessions.persistent.max": 0,
      "connections.max": 0,
      "sessions.persistent.count": 0,
      "actions.count": 5,
      "retained.count": 5,
      "rules.count": 0,
      "routes.count": 0,
      "subscriptions.shared.count": 0,
      "suboptions.count": 0,
      "sessions.count": 0,
      "actions.max": 5,
      "retained.max": 5,
      "sessions.max": 0,
      "rules.max": 0,
      "routes.max": 0,
      "resources.count": 0,
      "subscribers.count": 0,
      "connections.count": 0
    }
  ]
}
```
根据返回的数据，生成的指标信息
```
emqx_subscriptions_shared_max{node="xxx"}  123
emqx_subscriptions_max{node="xxx"} 123
emqx_subscribers_max{node="xxx"} 123
emqx_resources_max{node="xxx"} 123
emqx_topics_count{node="xxx"} 0
emqx_subscriptions_count{node="xxx"} 0
emqx_suboptions_max{node="xxx"} 0
emqx_topics_max{node="xxx"} 0
emqx_sessions_persistent_max{node="xxx"} 0
emqx_connections_max{node="xxx"} 0
emqx_sessions_persistent_count{node="xxx"} 0
emqx_actions_count{node="xxx"} 5
emqx_retained_count{node="xxx"} 5
emqx_rules_count{node="xxx"} 0
emqx_routes_count{node="xxx"} 0
emqx_subscriptions_shared_count{node="xxx"} 0
emqx_suboptions_count{node="xxx"} 0
emqx_sessions_count{node="xxx"} 0
emqx_actions_max{node="xxx"} 5
emqx_retained_max{node="xxx"} 5
emqx_sessions_max{node="xxx"} 0
emqx_rules_max{node="xxx"} 0
emqx_routes_max{node="xxx"} 0
emqx_resources_count{node="xxx"} 0
emqx_subscribers_count{node="xxx"} 0
emqx_connections_count{node="xxx"} 0
```

调用 `/api/${version}/nodes/${node}/metrics` 获取指标
```json
{
  "code": 0,
  "data": {
    "rules.matched": 0,
    "messages.sent": 0,
    "packets.disconnect.sent": 0,
    "bytes.sent": 0,
    "packets.disconnect.received": 0,
    "packets.pingresp.sent": 0,
    "packets.pingreq.received": 0,
    "packets.unsubscribe.received": 0,
    "packets.pubcomp.missed": 0,
    "packets.puback.missed": 0,
    "packets.pubcomp.sent": 0,
    "packets.pubcomp.received": 0,
    "packets.pubrec.missed": 0,
    "auth.mqtt.anonymous": 0,
    "packets.connack.auth_error": 0,
    "actions.failure": 0,
    "packets.suback.sent": 0,
    "packets.puback.sent": 0,
    "messages.retained": 5,
    "messages.received": 0,
    "packets.connect.received": 0,
    "messages.forward": 0,
    "packets.pubrel.missed": 0,
    "packets.publish.received": 0,
    "packets.connack.sent": 0,
    "packets.subscribe.received": 0,
    "packets.pubrel.received": 0,
    "packets.pubrec.received": 0,
    "packets.puback.received": 0,
    "packets.sent": 0,
    "packets.received": 0,
    "bytes.received": 0,
    "messages.expired": 0,
    "messages.dropped": 0,
    "messages.qos2.dropped": 0,
    "messages.qos2.expired": 0,
    "packets.pubrel.sent": 0,
    "packets.pubrec.sent": 0,
    "packets.publish.sent": 0,
    "actions.success": 0,
    "packets.publish.error": 0,
    "packets.unsubscribe.error": 0,
    "messages.qos2.received": 0,
    "messages.qos1.received": 0,
    "messages.qos0.received": 0,
    "packets.auth.sent": 0,
    "messages.qos2.sent": 0,
    "messages.qos1.sent": 0,
    "messages.qos0.sent": 0,
    "packets.auth.received": 0,
    "packets.unsuback.sent": 0,
    "packets.connack.error": 0,
    "packets.publish.auth_error": 0,
    "packets.subscribe.error": 0,
    "packets.subscribe.auth_error": 0
  }
}
```
对应生成的指标
```
emqx_rules_matched{node="xxx"} 0
emqx_messages_sent{node="xxx"} 0
emqx_packets_disconnect_sent{node="xxx"} 0
emqx_bytes_sent{node="xxx"} 0
emqx_packets_disconnect_received{node="xxx"} 0
emqx_packets_pingresp_sent{node="xxx"} 0
emqx_packets_pingreq_received{node="xxx"} 0
emqx_packets_unsubscribe_received{node="xxx"} 0
emqx_packets_pubcomp_missed{node="xxx"} 0
emqx_packets_puback_missed{node="xxx"} 0
emqx_packets_pubcomp_sent{node="xxx"} 0
emqx_packets_pubcomp_received{node="xxx"} 0
emqx_packets_pubrec_missed{node="xxx"} 0
emqx_auth_mqtt_anonymous{node="xxx"} 0
emqx_packets_connack_auth_error{node="xxx"} 0
emqx_actions_failure{node="xxx"} 0
emqx_packets_suback_sent{node="xxx"} 0
emqx_packets_puback_sent{node="xxx"} 0
emqx_messages_retained{node="xxx"} 5
emqx_messages_received{node="xxx"} 0
emqx_packets_connect_received{node="xxx"} 0
emqx_messages_forward{node="xxx"} 0
emqx_packets_pubrel_missed{node="xxx"} 0
emqx_packets_publish_received{node="xxx"} 0
emqx_packets_connack_sent{node="xxx"} 0
emqx_packets_subscribe_received{node="xxx"} 0
emqx_packets_pubrel_received{node="xxx"} 0
emqx_packets_pubrec_received{node="xxx"} 0
emqx_packets_puback_received{node="xxx"} 0
emqx_packets_sent{node="xxx"} 0
emqx_packets_received{node="xxx"} 0
emqx_bytes_received{node="xxx"} 0
emqx_messages_expired{node="xxx"} 0
emqx_messages_dropped{node="xxx"} 0
emqx_messages_qos2_dropped{node="xxx"} 0
emqx_messages_qos2_expired{node="xxx"} 0
emqx_packets_pubrel_sent{node="xxx"} 0
emqx_packets_pubrec_sent{node="xxx"} 0
emqx_packets_publish_sent{node="xxx"} 0
emqx_actions_success{node="xxx"} 0
emqx_packets_publish_error{node="xxx"} 0
emqx_packets_unsubscribe_error{node="xxx"} 0
emqx_messages_qos2_received{node="xxx"} 0
emqx_messages_qos1_received{node="xxx"} 0
emqx_messages_qos0_received{node="xxx"} 0
emqx_packets_auth_sent{node="xxx"} 0
emqx_messages_qos2_sent{node="xxx"} 0
emqx_messages_qos1_sent{node="xxx"} 0
emqx_messages_qos0_sent{node="xxx"} 0
emqx_packets_auth_received{node="xxx"} 0
emqx_packets_unsuback_sent{node="xxx"} 0
emqx_packets_connack_error{node="xxx"} 0
emqx_packets_publish_auth_error{node="xxx"} 0
emqx_packets_subscribe_error{node="xxx"} 0
emqx_packets_subscribe_auth_error{node="xxx"} 0
```

3. 其他
   5.X的emqx提供prometheus接口 ，可以使用input.prometheus插件配置`http://ip:18083/api/v5/prometheus/stats` 采集, 不需要额外配置用户名密码认。