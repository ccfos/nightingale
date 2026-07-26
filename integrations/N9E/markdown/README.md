# N9E

夜莺 V5 的 n9e-webapi、n9e-server，以及 V6–V9 的 n9e 进程都通过 `/metrics` 暴露 Prometheus 指标。边缘机房使用的 n9e-edge 也提供 `/metrics`。

本次使用 Nightingale V9 自监控端点完成验证。

## 采集配置

新建 Categraf 配置 `conf/input.prometheus/n9e.toml`：

```toml
[[instances]]
urls = ["http://127.0.0.1:17000/metrics"]
url_label_key = "instance"
url_label_value = "{{.Host}}"
labels = { job = "n9e" }
```

如果有多个 n9e 或 n9e-edge 实例，应全部配置，并确保 `instance` 唯一。先用下面的命令确认服务端确实暴露指标：

```bash
curl -fsS http://127.0.0.1:17000/metrics | head
```
