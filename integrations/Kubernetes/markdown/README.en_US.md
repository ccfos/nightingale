# Kubernetes

This plugin is deprecated. For Kubernetes monitoring, refer to this [article series](https://flashcat.cloud/categories/kubernetes%E7%9B%91%E6%8E%A7%E4%B8%93%E6%A0%8F/).

However, the built-in alert rules and built-in dashboards under the Kubernetes category are still usable.

---

Below is the documentation of the old plugin:

forked from telegraf/kubernetes. This plugin fetches monitoring data through the API provided by kubelet, including metrics for system containers, nodes, pod volumes, pod networks, and pod containers.

## Change

Several control switches were added:

`gather_system_container_metrics = true`

Whether to collect system containers (kubelet, runtime, misc, pods). For example, kubelet is generally a static container, not a business container.

`gather_node_metrics = true`

Whether to collect node-level metrics. Machine-level metrics are actually already collected by categraf, so in theory there is no need to collect them again here; you can set this to false. Collecting them is also fine — the data volume is small.

`gather_pod_container_metrics = true`

Whether to collect metrics of containers inside Pods; these Pods are generally business containers.

`gather_pod_volume_metrics = true`

Whether to collect metrics of Pod volumes.

`gather_pod_network_metrics = true`

Whether to collect Pod network monitoring data.

## Container monitoring

As these switches show, the kubernetes plugin only collects pod and container monitoring metrics, and the data comes from kubelet endpoints such as `/stats/summary` and `/pods`. So the question arises: for container monitoring, should you read the `/metrics/cadvisor` endpoint or use this kubernetes plugin? Here are a few decision criteria:

1. Data collected from `/metrics/cadvisor` has no custom business labels, while the kubernetes plugin automatically attaches custom business labels. However, business labels can get messy, so each company is advised to establish a convention, e.g. requiring that business teams only use labels like project, region, env, service, app, and job, and filtering out all others. Label filtering can be done via the plugin's label_include and label_exclude settings.
2. The kubernetes plugin collects fewer metrics than what `/metrics/cadvisor` exposes, but the common ones related to cpu, mem, net, and volume are all covered.
