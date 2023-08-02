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

完整的带有注释的配置如下：

```toml
[[instances]]
targets = [
#     "http://localhost",
#     "https://www.baidu.com"
]

# # append some labels for series
# labels = { region="cloud", product="n9e" }

# # interval = global.interval * interval_times
# interval_times = 1

## Set http_proxy (categraf uses the system wide proxy settings if it's is not set)
# http_proxy = "http://localhost:8888"

## Interface to use when dialing an address
# interface = "eth0"

## HTTP Request Method
# method = "GET"

## Set response_timeout (default 5 seconds)
# response_timeout = "5s"

## Whether to follow redirects from the server (defaults to false)
# follow_redirects = false

## Optional HTTP Basic Auth Credentials
# username = "username"
# password = "pa$$word"

## Optional headers
# headers = ["Header-Key-1", "Header-Value-1", "Header-Key-2", "Header-Value-2"]

## Optional HTTP Request Body
# body = '''
# {'fake':'data'}
# '''

## Optional substring match in body of the response (case sensitive)
# expect_response_substring = "ok"

## Optional expected response status code.
# expect_response_status_code = 0

## Optional TLS Config
# use_tls = false
# tls_ca = "/etc/categraf/ca.pem"
# tls_cert = "/etc/categraf/cert.pem"
# tls_key = "/etc/categraf/key.pem"
## Use TLS but skip chain & host verification
# insecure_skip_verify = false
```

## dashboard and monitors

夜莺提供了内置大盘和内置告警规则，克隆到自己的业务组下即可使用。