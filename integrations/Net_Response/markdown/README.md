# net_response

网络探测插件，通常用于监控本机某个端口是否在监听，或远端某个端口是否能连通

## code meanings

- 0: Success
- 1: Timeout
- 2: ConnectionFailed
- 3: ReadFailed
- 4: StringMismatch

## Configuration

最核心的配置就是 targets 部分，指定探测的目标，下面的例子：

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

监控数据或告警事件中只是一个 IP 和端口，接收告警的人看到了，可能不清楚只是哪个业务的模块告警了，可以附加一些更有价值的信息放到标签里，比如例子中：

```toml
labels = { region="cloud", product="n9e" }
```

标识了这是 cloud 这个 region，n9e 这个产品，这俩标签会附到时序数据上，告警的时候自然也会报出来。

## 监控大盘和告警规则

该 README 的同级目录下，提供了 dashboard.json 就是监控大盘的配置，alerts.json 是告警规则，可以导入夜莺使用。