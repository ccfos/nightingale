# Kubernetes

旧的 Categraf Kubernetes input 已废弃，但本目录的仪表盘和告警规则仍可使用。推荐通过 Categraf Prometheus input 抓取 Kubernetes 各组件的原生 `/metrics`，并部署 kube-state-metrics 和 node-exporter。

## 模板所需数据源

| 数据源 | 常用端点 | 覆盖模板 |
| --- | --- | --- |
| kube-apiserver | `https://<api-server>:6443/metrics` | API Server |
| kubelet | `https://<node>:10250/metrics`、`/metrics/cadvisor`、`/metrics/resource` | Kubelet、Node、Pod、Container |
| kube-controller-manager | `https://<control-plane>:10257/metrics` | Controller Manager |
| kube-scheduler | `https://<control-plane>:10259/metrics` | Scheduler |
| kube-proxy | `http://<node>:10249/metrics` | Proxy |
| kube-state-metrics | `http://<service>:8080/metrics` | Deployment、StatefulSet、DaemonSet、Pod 等状态 |
| CoreDNS | `http://<coredns>:9153/metrics` | CoreDNS |
| node-exporter | `http://<node>:9100/metrics` | Node 主机资源 |

本次使用单节点 K3s 实测。K3s 的 scheduler、controller-manager 和 proxy 指标可能只监听 loopback，需要在控制面节点本机采集，或使用受控代理暴露；不要为了方便把这些无鉴权端点开放到公网。

## Categraf 示例

```toml
# conf/input.prometheus/kubernetes.toml
interval = 15

[[instances]]
urls = ["https://kubernetes.default.svc:443/metrics"]
bearer_token_file = "/var/run/secrets/kubernetes.io/serviceaccount/token"
use_tls = true
tls_ca = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
labels = { cluster = "prod-k8s", job = "apiserver" }

[[instances]]
urls = [
  "https://<node>:10250/metrics",
  "https://<node>:10250/metrics/cadvisor",
  "https://<node>:10250/metrics/resource"
]
bearer_token_file = "/path/to/token"
use_tls = true
tls_ca = "/path/to/ca.crt"
labels = { cluster = "prod-k8s", job = "kubelet" }

[[instances]]
urls = ["http://kube-state-metrics.monitoring.svc:8080/metrics"]
labels = { cluster = "prod-k8s", job = "kube-state-metrics" }

[[instances]]
urls = ["http://node-exporter.monitoring.svc:9100/metrics"]
labels = { cluster = "prod-k8s", job = "node-exporter" }
```

所有数据源应使用相同且稳定的 `cluster` 标签，并保留每个目标唯一的 `instance`。生产环境应校验证书，不建议使用 `insecure_skip_verify = true`。

单节点或非高可用集群没有某些角色、runtime、设备或副本时，相关面板为空属于拓扑差异。

---

下面保留旧插件说明，仅供已有部署参考：

forked from telegraf/kubernetes. 这个插件的作用是通过kubelet提供的API获取监控数据，包括系统容器的监控数据、node的、pod数据卷的、pod网络的、pod容器的。

## Change

增加了一些控制开关：

`gather_system_container_metrics = true`

是否采集 system 容器（kubelet、runtime、misc、pods），比如 kubelet 一般就是静态容器，非业务容器

`gather_node_metrics = true`

是否采集 node 层面的指标，机器层面的指标其实 categraf 来采集了，这里理论上不需要再采集了，可以设置为 false，采集也没问题，也没多少数据

`gather_pod_container_metrics = true`

是否采集 Pod 中的容器的指标，这些 Pod 一般是业务容器

`gather_pod_volume_metrics = true`

是否采集 Pod 的数据卷的指标

`gather_pod_network_metrics = true`

是否采集 Pod 的网络监控数据

## 容器监控

通过这些开关可以看出，kubernetes 这个插件，采集的只是 pod、容器的监控指标，这些指标数据来自 kubelet 的 `/stats/summary` `/pods` 等接口。那么问题来了，容器监控到底是应该读取 `/metrics/cadvisor` 接口还是应该用这个 kubernetes 插件？有几个决策依据：

1. `/metrics/cadvisor` 采集的数据没有业务自定义标签，kubernetes 这个插件会自动带上业务自定义标签。但是业务标签可能比较混乱，建议每个公司制定规范，比如要求业务只能打 project、region、env、service、app、job 等标签，其他标签都过滤掉，通过 kubernetes 插件的 label_include label_exclude 配置，可以做标签过滤。
2. kubernetes 这个插件采集的数据比 `/metrics/cadvisor` 吐出的指标要少，不过常见的 cpu、mem、net、volume 相关的也都有。
