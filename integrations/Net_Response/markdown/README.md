# net_response

网络探测插件，通常用于监控本机某个端口是否在监听，或远端某个端口是否能连通。因为 Prometheus 生态的时序库只能存储 float64 类型的值，所以网络探测插件探测的结果也是 float64 类型的值，但是这个值的含义是不同的，具体含义如下：

```
- 0: Success
- 1: Timeout
- 2: ConnectionFailed
- 3: ReadFailed
- 4: StringMismatch
```

如果一切正常，这个值是 0，如果有异常，这个值是 1-4 之间的值，具体含义如上。这个值对应的指标名字是 `net_response_result_code`。

## Configuration

categraf 的 `conf/input.net_response/net_response.toml`。最核心的配置就是 targets 部分，指定探测的目标，下面的例子：

```toml
[[instances]]
targets = [
    "10.2.3.4:22",
    "localhost:6379",
    ":9090"
]
```

- `10.2.3.4:22` 表示探测 10.2.3.4 这个机器的 22 端口是否可以连通
- `localhost:6379` 表示探测本机的 6379 端口是否可以连通
- `:9090` 表示探测本机的 9090 端口是否可以连通

监控数据或告警事件中只是一个 IP 和端口，接收告警的人看到了，可能不清楚只是哪个业务的模块告警了，可以附加一些更有价值的信息放到标签里，比如：

```toml
labels = { region="cloud", product="n9e" }
```

标识了这是 cloud 这个 region，n9e 这个产品，这俩标签会附到时序数据上，告警的时候自然也会报出来。

完整配置样例如下：

```toml
[mappings]
# "127.0.0.1:22"= {region="local",ssh="test"}
# "127.0.0.1:22"= {region="local",ssh="redis"}

[[instances]]
targets = [
#     "127.0.0.1:22",
#     "localhost:6379",
#     ":9090"
]

# # append some labels for series
# labels = { region="cloud", product="n9e" }

# # interval = global.interval * interval_times
# interval_times = 1

## Protocol, must be "tcp" or "udp"
## NOTE: because the "udp" protocol does not respond to requests, it requires
## a send/expect string pair (see below).
# protocol = "tcp"

## Set timeout
# timeout = "1s"

## Set read timeout (only used if expecting a response)
# read_timeout = "1s"

## The following options are required for UDP checks. For TCP, they are
## optional. The plugin will send the given string to the server and then
## expect to receive the given 'expect' string back.
## string sent to the server
# send = "ssh"
## expected string in answer
# expect = "ssh"
```

## 监控大盘和告警规则

夜莺内置了仪表盘和告警规则，克隆到自己的业务组即可使用。