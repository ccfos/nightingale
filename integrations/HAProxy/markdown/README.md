# HAProxy

本模板已使用 Categraf 的 HAProxy input 对真实 HAProxy 后端流量完成验证。该 input 可以读取 HAProxy CSV stats 页面或 Runtime API socket。

## 开启 HAProxy stats

```haproxy
frontend stats
    bind *:8404
    stats enable
    stats uri /stats
    stats refresh 10s
```

先确认 CSV 输出可访问：

```bash
curl -fsS 'http://127.0.0.1:8404/stats;csv' | head
```

## Categraf 配置

配置文件为 `conf/input.haproxy/haproxy.toml`：

```toml
[[instances]]
uri = "http://127.0.0.1:8404/stats;csv"
ssl_verify = true
timeout = "5s"
```

也可以使用 socket：

```toml
[[instances]]
uri = "unix:/run/haproxy/admin.sock"
timeout = "5s"
```

HAProxy 2.0 及以上也可以编译原生 Prometheus exporter，由 Categraf Prometheus input 抓取；但本目录的 `HAProxy By Categraf` 模板是按 Categraf HAProxy input 的指标名验证的。应持续向 frontend/backend 产生真实请求，否则速率和响应码面板可能为空。
