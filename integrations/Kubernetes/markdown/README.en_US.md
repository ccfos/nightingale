# Kubernetes

The legacy Categraf Kubernetes input is deprecated, but the dashboards and
alerts remain usable. Collect native Prometheus endpoints with Categraf and
deploy kube-state-metrics and node-exporter.

Required sources include:

| Source | Common endpoint |
| --- | --- |
| kube-apiserver | `https://<api-server>:6443/metrics` |
| kubelet | `https://<node>:10250/metrics`, `/metrics/cadvisor`, `/metrics/resource` |
| controller-manager | `https://<control-plane>:10257/metrics` |
| scheduler | `https://<control-plane>:10259/metrics` |
| kube-proxy | `http://<node>:10249/metrics` |
| kube-state-metrics | `http://<service>:8080/metrics` |
| CoreDNS | `http://<coredns>:9153/metrics` |
| node-exporter | `http://<node>:9100/metrics` |

Example:

```toml
interval = 15

[[instances]]
urls = ["https://kubernetes.default.svc:443/metrics"]
bearer_token_file = "/var/run/secrets/kubernetes.io/serviceaccount/token"
use_tls = true
tls_ca = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
labels = { cluster = "prod-k8s", job = "apiserver" }

[[instances]]
urls = ["http://kube-state-metrics.monitoring.svc:8080/metrics"]
labels = { cluster = "prod-k8s", job = "kube-state-metrics" }
```

Use one stable `cluster` label across all targets and preserve a unique
`instance`. K3s may bind scheduler, controller-manager, and proxy metrics to
loopback; collect them locally or through a restricted proxy. Do not expose
unauthenticated control-plane metrics publicly. Missing roles and HA-only
features legitimately produce empty panels in a single-node cluster.

---

The legacy plugin documentation follows for existing deployments:

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
