# kafka plugin

Kafka 的核心指标，其实都是通过 JMX 的方式暴露的，可以参考这篇 [文章](https://time.geekbang.org/column/article/628498)。对于 JMX 暴露的指标，使用 jolokia 或者使用 jmx_exporter 那个 jar 包来采集即可，不需要本插件。

本插件主要是采集的消费者延迟数据，这个数据无法通过 Kafka 服务端的 JMX 拿到。

本插件 fork 自 [https://github.com/davidmparrott/kafka_exporter](https://github.com/davidmparrott/kafka_exporter)（以下简称 davidmparrott 版本），davidmparrott 版本 fork 自 [https://github.com/danielqsj/kafka_exporter](https://github.com/danielqsj/kafka_exporter)（以下简称 danielqsj 版本）。

danielqsj 版本作为原始版本, github 版本也相对活跃, prometheus 生态使用较多。davidmparrott 版本与 danielqsj 版本相比, 有以下 metric 名字不同：

| davidmparrott 版本  | danielqsj 版本 |
| ---- | ---- |
| kafka_consumergroup_uncommit_offsets  | kafka_consumergroup_lag |
| kafka_consumergroup_uncommit_offsets_sum  | kafka_consumergroup_lag_sum |
| kafka_consumergroup_uncommitted_offsets_zookeeper | kafka_consumergroup_lag_zookeeper |

如果想使用 danielqsj 版本的 metric, 在 `[[instances]]` 中进行如下配置:

```toml
rename_uncommit_offset_to_lag = true
```

davidmparrott 版本比 danielqsj 版本多了以下 metric，这些指标是对延迟速率做了预估计算：

- kafka_consumer_lag_millis
- kafka_consumer_lag_interpolation
- kafka_consumer_lag_extrapolation

为什么要计算速率？因为 lag 很大，但是消费很快，是不会积压的，而 lag 很小，消费很慢，仍然会积压，所以，通过 lag 大小是没法判断积压风险的。通过计算历史消费速率，来判断积压风险会更为合理。要计算这个速率，需要占用较多内存，可以通过如下配置关闭这个计算逻辑：

```toml
disable_calculate_lag_rate = true
```

## 采集配置

categraf 配置文件：`conf/input.kafka/kafka.toml`。配置样例如下：

```toml
[[instances]]
log_level = "error"
kafka_uris = ["192.168.0.250:9092"]
labels = { cluster="kafka-cluster-01", service="kafka" }
```

完整的带有注释的配置如下：

```toml
[[instances]]
# # interval = global.interval * interval_times
# interval_times = 1

# append some labels to metrics
# cluster is a preferred tag with the cluster name. If none is provided, the first of kafka_uris will be used
labels = { cluster="kafka-cluster-01" }

# log level only for kafka exporter
log_level = "error"

# Address (host:port) of Kafka server.
# kafka_uris = ["127.0.0.1:9092","127.0.0.1:9092","127.0.0.1:9092"]
kafka_uris = []

# Connect using SASL/PLAIN
# Default is false
# use_sasl = false

# Only set this to false if using a non-Kafka SASL proxy
# Default is true
# use_sasl_handshake = false

# SASL user name
# sasl_username = "username"

# SASL user password
# sasl_password = "password"

# The SASL SCRAM SHA algorithm sha256 or sha512 as mechanism
# sasl_mechanism = ""

# Connect using TLS
# use_tls = false

# The optional certificate authority file for TLS client authentication
# ca_file = ""

# The optional certificate file for TLS client authentication
# cert_file = ""

# The optional key file for TLS client authentication
# key_file = ""

# If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
# insecure_skip_verify = true

# Kafka broker version
# Default is 2.0.0
# kafka_version = "2.0.0"

# if you need to use a group from zookeeper
# Default is false
# use_zookeeper_lag = false

# Address array (hosts) of zookeeper server.
# zookeeper_uris = []

# Metadata refresh interval
# Default is 1m
# metadata_refresh_interval = "1m"

# Whether show the offset/lag for all consumer group, otherwise, only show connected consumer groups, default is true
# Default is true
# offset_show_all = true

# If true, all scrapes will trigger kafka operations otherwise, they will share results. WARN: This should be disabled on large clusters
# Default is false
# allow_concurrency = false

# Maximum number of offsets to store in the interpolation table for a partition
# Default is 1000
# max_offsets = 1000

# How frequently should the interpolation table be pruned, in seconds.
# Default is 30
# prune_interval_seconds = 30

# Regex filter for topics to be monitored
# Default is ".*"
# topics_filter_regex = ".*"

# Regex filter for consumer groups to be monitored
# Default is ".*"
# groups_filter_regex = ".*"

# if rename  kafka_consumergroup_uncommitted_offsets to kafka_consumergroup_lag
# Default is false
# rename_uncommit_offset_to_lag = false


# if disable calculating lag rate
# Default is false
# disable_calculate_lag_rate = false
```

## 告警规则

夜莺提供了内置的 Kafka 告警规则，克隆到自己的业务组下即可使用。

![20230801162030](https://download.flashcat.cloud/ulric/20230801162030.png)

## 仪表盘：

夜莺提供了内置的 Kafka 仪表盘，克隆到自己的业务组下即可使用。

![20230801162017](https://download.flashcat.cloud/ulric/20230801162017.png)
