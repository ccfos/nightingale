# http_response plugin

HTTP 探测插件，用于检测 HTTP 地址的连通性、延迟、HTTPS 证书过期时间。因为 Prometheus 生态的时序库只能存储 float64 类型的值，所以 HTTP 地址探测的结果也是 float64 类型的值，但是这个值的含义是不同的，具体含义如下：

```
Success          = 0
ConnectionFailed = 1
Timeout          = 2
DNSError         = 3
AddressError     = 4
BodyMismatch     = 5
CodeMismatch     = 6
```

如果一切正常，这个值是 0，如果有异常，这个值是 1-6 之间的值，具体含义如上。这个值对应的指标名字是 `http_response_result_code`。

## Configuration

categraf 的 `conf/input.http_response/http_response.toml`。最核心的配置就是 targets 配置，配置目标地址，比如想要监控两个地址：

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

完整的带有注释的配置文件，请参考 [这里](https://github.com/flashcatcloud/categraf/blob/main/conf/input.http_response/http_response.toml)。

## dashboard and monitors

夜莺提供了内置大盘和内置告警规则，克隆到自己的业务组下即可使用。