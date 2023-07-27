# http_response

HTTP 探测插件，用于检测 HTTP 地址的连通性、延迟、HTTPS证书过期时间

## code meanings

```
Success          = 0
ConnectionFailed = 1
Timeout          = 2
DNSError         = 3
AddressError     = 4
BodyMismatch     = 5
CodeMismatch     = 6
```

## Configuration

最核心的配置就是 targets 配置，配置目标地址，比如想要监控两个地址：

```toml
[[instances]]
targets = [
    "http://localhost:8080",
    "https://www.baidu.com"
]
```

instances 下面的所有 targets 共享同一个 `[[instances]]` 下面的配置，比如超时时间，HTTP方法等，如果有些配置不同，可以拆成多个不同的 `[[instances]]`，比如：

```toml
[[instances]]
targets = [
    "http://localhost:8080",
    "https://www.baidu.com"
]
method = "GET"

[[instances]]
targets = [
    "http://localhost:9090"
]
method = "POST"
```

## 监控大盘和告警规则

该 README 的同级目录下，提供了 dashboard.json 就是监控大盘的配置，alerts.json 是告警规则，可以导入夜莺使用。