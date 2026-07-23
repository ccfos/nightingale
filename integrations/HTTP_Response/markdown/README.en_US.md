# http_response plugin

An HTTP probing plugin used to check the connectivity and latency of HTTP endpoints, as well as HTTPS certificate expiration time. Since time series databases in the Prometheus ecosystem can only store float64 values, the HTTP probe result is also a float64 value, but the value carries different meanings, as follows:

```
Success          = 0
ConnectionFailed = 1
Timeout          = 2
DNSError         = 3
AddressError     = 4
BodyMismatch     = 5
CodeMismatch     = 6
```

If everything is fine, the value is 0; if something goes wrong, the value is between 1 and 6, with the meanings shown above. The metric name for this value is `http_response_result_code`.

## Configuration

categraf's `conf/input.http_response/http_response.toml`. The most important setting is `targets`, which configures the target endpoints. For example, to monitor two endpoints:

```toml
[[instances]]
targets = [
    "http://localhost:8080",
    "https://www.baidu.com"
]
```

All targets under one `[[instances]]` section share the settings of that `[[instances]]` section, such as the timeout, HTTP method, etc. If some settings differ, split them into multiple `[[instances]]` sections, for example:

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

The full configuration with comments is as follows:

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
