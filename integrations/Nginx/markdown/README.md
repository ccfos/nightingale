# Nginx

Nginx 监控有多种方式，最推荐的是 vts 插件：

**[http_stub_status_module](https://github.com/flashcatcloud/categraf/blob/main/inputs/nginx/README.md)**

配置样例如下：

```toml
[[instances]]
## An array of Nginx stub_status URI to gather stats.
urls = [
#    "http://192.168.0.216:8000/nginx_status",
#    "https://www.baidu.com/ngx_status"
]

## append some labels for series
# labels = { region="cloud", product="n9e" }

## interval = global.interval * interval_times
# interval_times = 1

## Set response_timeout (default 5 seconds)
response_timeout = "5s"

## Whether to follow redirects from the server (defaults to false)
# follow_redirects = false

## Optional HTTP Basic Auth Credentials
#username = "admin"
#password = "admin"

## Optional headers
# headers = ["X-From", "categraf", "X-Xyz", "abc"]

## Optional TLS Config
# use_tls = false
# tls_ca = "/etc/categraf/ca.pem"
# tls_cert = "/etc/categraf/cert.pem"
# tls_key = "/etc/categraf/key.pem"
## Use TLS but skip chain & host verification
# insecure_skip_verify = false
```

**[nginx_upstream_check](https://github.com/flashcatcloud/categraf/blob/main/inputs/nginx_upstream_check/README.md)**

配置样例如下：

```toml
[[instances]]
targets = [
    # "http://127.0.0.1/status?format=json",
    # "http://10.2.3.56/status?format=json"
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

## Set timeout (default 5 seconds)
# timeout = "5s"

## Whether to follow redirects from the server (defaults to false)
# follow_redirects = false

## Optional HTTP Basic Auth Credentials
# username = "username"
# password = "pa$$word"

## Optional headers
# headers = ["X-From", "categraf", "X-Xyz", "abc"]

## Optional TLS Config
# use_tls = false
# tls_ca = "/etc/categraf/ca.pem"
# tls_cert = "/etc/categraf/cert.pem"
# tls_key = "/etc/categraf/key.pem"
## Use TLS but skip chain & host verification
# insecure_skip_verify = false
```

**[nginx vts](https://github.com/flashcatcloud/categraf/blob/main/inputs/nginx_vts/README.md)**

nginx_vts 已经支持输出 prometheus 格式的数据，所以，其实已经不需要这个采集插件了，直接用 categraf 的 prometheus 采集插件，读取 nginx_vts 的 prometheus 数据即可。配置样例如下：

```toml
[[instances]]
urls = [
  "http://IP:PORT/vts/format/prometheus"
]
labels = {job="nginx-vts"}
```

## 仪表盘

夜莺内置了相关仪表盘，克隆到自己的业务组即可使用。