# Kubernetes

这个插件已经废弃。Kubernetes 监控系列可以参考这个 [文章](https://flashcat.cloud/categories/kubernetes%E7%9B%91%E6%8E%A7%E4%B8%93%E6%A0%8F/)。或者参考 [专栏](https://time.geekbang.org/column/article/630306)。

不过 Kubernetes 这个类别下的内置告警规则和内置仪表盘都是可以使用的。

---

下面是老插件文档：

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
