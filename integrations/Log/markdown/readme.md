# 日志采集

## 概述
日志采集方式：
- **文件采集**：采集本地文件系统中的日志文件
- **Pod 日志采集**：采集 Kubernetes Pod 的 stdout/stderr 日志
- **Rsyslog 日志采集**：通过 udp/tcp 等协议采集Rsyslog日志

日志推送方式：
- **n9e**：categraf 将日志上报到 flashcat 服务端，由服务端配置转发到哪个 kafka
- **Kafka**: 如果选择此方式，可以在左侧配置选择将日志写到哪个 kafka 数据源

## 注意
日志采集功能，需要企业版 categraf 采集器的最低版本是 **v0.4.48**

---

## 配置格式

配置文件使用 TOML 格式，每个采集项通过 `[[items]]` 定义。

---

## 一、文件采集

### 基本配置

```toml
[[items]]
type = "file"
path = "/opt/app/logs/*.log"
source = "payment-service"
service = "payment"
topic = "payment-logs"
```

### 参数说明

| 参数 | 必填 | 说明                                         |
|------|------|--------------------------------------------|
| `type` | 是 | 固定值 `"file"`，表示文件采集模式                      |
| `path` | 是 | 日志文件路径，**支持通配符**（如 `*.log`、`access-*.log`） |
| `source` | 是 | 自定义，发送到 Kafka 时的 `source` 标签，用于标识日志来源      |
| `service` | 是 | 自定义，发送到 Kafka 时的 `service` 标签，用于标识服务名称     |
| `topic` | 是 | 日志发送到 Kafka 的目标 Topic                      |

---

## 二、Pod 日志采集

### 基本配置

```toml
[[items]]
type = "pod"
source = "K8s"
service = "my-service"
container_logs_parser = "containerd"
```

### 参数说明

| 参数 | 必填 | 说明 |
|------|------|------|
| `type` | 是 | 固定值 `"pod"`，表示 Pod 日志采集模式 |
| `source` | 是 | 自定义，发送到 Kafka 时的 `source` 标签，用于标识日志来源 |
| `service` | 是 | 自定义，发送到 Kafka 时的 `service` 标签，用于标识服务名称  |
| `container_logs_parser` | 是 | K8s 容器运行时日志格式，如 `containerd`、`docker`、`podman` |

---

## 三、Rsyslog 日志采集

### 基本配置

```toml
[[items]]
type = "udp"
port = 514
source = "rsyslog"
service = "logs collector"
```

### 参数说明

| 参数 | 必填 | 说明                                       |
|------|------|------------------------------------------|
| `type` | 是 | 固定值 `"udp"` 或 `"tcp"`，表示 Rsyslog日志采集网络协议 |
| `port` | 是 | 监听的端口                                    |
| `source` | 是 | 自定义，发送到 Kafka 时的 `source` 标签，用于标识日志来源    |
| `service` | 是 | 自定义，发送到 Kafka 时的 `service` 标签，用于标识服务名称    |
---

## 四、过滤规则（filter_rules）

过滤规则用于筛选需要采集的 Pod，配置方式类似 Prometheus 的 relabel 机制。

### 基本语法

```toml
[[items.filter_rules]]
source_labels = ["<标签名>"]
regex = "<正则表达式>"
action = "keep" | "drop"
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `source_labels` | 匹配的标签来源，支持多个标签 |
| `regex` | 正则表达式，用于匹配标签值 |
| `action` | `keep`：保留匹配的 Pod；`drop`：丢弃匹配的 Pod |

### 支持的 source_labels 类型

| 类型 | 说明 | 示例 |
|------|------|------|
| `pod_name` | Pod 名称 | `source_labels=["pod_name"]` |
| `pod_namespace` | Pod 所在命名空间 | `source_labels=["pod_namespace"]` |
| `image_name` | 容器镜像名称 | `source_labels=["image_name"]` |
| `label_<key>` | Pod Label | `source_labels=["label_app"]` |
| `annotation_<key>` | Pod Annotation | `source_labels=["annotation_k8s_io_pod-ips"]` |

> **注意**：对于 Label 和 Annotation，如果原始 key 中包含 `/`、`:`、`.` 字符，需要手动替换为 `_`

### 示例

#### 1. 按 Pod 名称过滤

```toml
[[items]]
type = "pod"
source = "K8s"
service = "my-service"
container_logs_parser = "containerd"

[[items.filter_rules]]
source_labels = ["pod_name"]
regex = "nightingale-center-7cc66cb9cc.*"
action = "keep"
```

#### 2. 按 Pod Label 过滤

采集包含 `app: n9e` 标签的所有 Pod：

```toml
[[items.filter_rules]]
source_labels = ["label_app"]
regex = "n9e"
action = "keep"
```

#### 3. 按 Pod Annotation 过滤

采集包含 `k8s.io/pod-ips: 10.99.1.219` 注解的 Pod：

```toml
[[items.filter_rules]]
# 原始 key "k8s.io/pod-ips" 中的 "/" 和 "." 替换为 "_"
source_labels = ["annotation_k8s_io_pod-ips"]
regex = "10.99.1.219"
action = "keep"
```

---

## 五、日志处理规则（log_processing_rules）

### 多行日志匹配

对于 Java 堆栈等多行日志，可以配置多行合并规则：

```toml
[[items]]
type = "pod"
source = "multiline-test"
service = "java-service"
container_logs_parser = "containerd"

# 先匹配要采集的 Pod
[[items.filter_rules]]
source_labels = ["pod_name"]
regex = "^multiline-log-test.*"
action = "keep"

# 配置多行规则
[[items.log_processing_rules]]
type = "multi_line"
name = "java_start_line"
pattern = "^\\d{4}-\\d{2}-\\d{2}"
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `type` | 规则类型，`multi_line` 表示多行匹配 |
| `name` | 规则名称，自定义标识 |
| `pattern` | 正则表达式，匹配新日志行的**起始特征** |

> **说明**：上述示例中 `pattern = "^\\d{4}-\\d{2}-\\d{2}"` 表示以日期（如 `2024-01-01`）开头的行为新日志的起始行，之前不匹配的行会被合并到上一条日志中。

---

## 六、完整配置示例

```toml
# 采集文件日志示例
[[items]]
type = "file"
path = "/opt/app/logs/*.log"
source = "payment-service"
service = "payment"
topic = "payment-logs"
```

```toml
# 采集Pod 日志示例（带过滤和多行处理）
[[items]]
type = "pod"
source = "java-app"
service = "order-service"
container_logs_parser = "containerd"

[[items.filter_rules]]
source_labels = ["pod_namespace"]
regex = "production"
action = "keep"

[[items.filter_rules]]
source_labels = ["label_app"]
regex = "order-.*"
action = "keep"

[[items.log_processing_rules]]
type = "multi_line"
name = "java_exception"
pattern = "^\\d{4}-\\d{2}-\\d{2}"
```

---

## 七、常见问题

### Q1: 如何排除某些 Pod？
使用 `action = "drop"` 配合正则匹配要排除的 Pod。

### Q2: 多个 filter_rules 之间是什么关系？
多个 filter_rules 之间是 **AND** 关系，需要同时满足所有规则。

### Q3: 通配符路径如何使用？
文件采集的 `path` 支持 `*` 通配符，如 `/var/log/*.log` 会匹配该目录下所有 `.log` 结尾的文件。

