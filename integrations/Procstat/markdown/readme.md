# 进程监控

使用 categraf procstat 插件。

## 配置文件

位置：categraf 的 `conf/input.procstat/procstat.toml`

样例配置：

```toml
[[instances]]
# # executable name (ie, pgrep <search_exec_substring>)
search_exec_substring = "nginx"

# # pattern as argument for pgrep (ie, pgrep -f <search_cmdline_substring>)
# search_cmdline_substring = "n9e server"

# # windows service name
# search_win_service = ""

# # search process with specific user, option with exec_substring or cmdline_substring
# search_user = ""

# # append some labels for series
# labels = { region="cloud", product="n9e" }

# # interval = global.interval * interval_times
# interval_times = 1

# # mode to use when calculating CPU usage. can be one of 'solaris' or 'irix'
# mode = "irix"

# sum of threads/fd/io/cpu/mem, min of uptime/limit
gather_total = true

# will append pid as tag
gather_per_pid = false

#  gather jvm metrics only when jstat is ready
# gather_more_metrics = [
#     "threads",
#     "fd",
#     "io",
#     "uptime",
#     "cpu",
#     "mem",
#     "limit",
#     "jvm"
# ]
```

机器上有很多进程，要监控进程是否存活以及进程的资源占用，首先得告诉 categraf，要监控的进程是啥。所以，本插件一开始的几个配置，就是做进程过滤的，用来告诉 categraf 要监控的进程是哪些。

- search_exec_substring 配置一个查询字符串，相当于执行 `pgrep <search_exec_substring>`
- search_cmdline_substring 配置一个查询字符串，相当于执行 `pgrep -f <search_cmdline_substring>`
- search_win_service 配置一个 windows 服务名，相当于执行 `sc query <search_win_service>`

上例默认是采集 nginx。默认只会采集一个指标：procstat_lookup_count，表示通过这些过滤条件，查询到的进程的数量。那显然，如果 `procstat_lookup_count <= 0` 就说明进程不存在了。

## CPU 利用率计算

在计算 CPU 利用率的时候有两种模式：irix（默认）、solaris。如果是 irix 模式，CPU 利用率会出现大于 100% 的情况，如果是 solaris 模式，会考虑 CPU 核数，所以 CPU 利用率不会大于 100%。

## 采集更多指标

`gather_more_metrics` 默认没有打开，即不会采集进程资源利用情况。如果想要采集，就打开 `gather_more_metrics` 这个配置即可。其中最为特殊的是 `jvm`，如果想要采集 jvm 指标，需要先安装好 jstat，然后再打开 `jvm` 这个配置。

## gather_total

比如进程名字是 mysql 的进程，同时可能运行了多个，我们想知道这个机器上的所有 mysql 的进程占用的总的 cpu、mem、fd 等，就设置 gather_total = true，当然，对于 uptime 和 limit 的采集，gather_total 的时候是取的多个进程的最小值。

## gather_per_pid

还是拿 mysql 举例，一个机器上可能同时运行了多个，我们可能想知道每个 mysql 进程的资源占用情况，此时就要启用 gather_per_pid 的配置，设置为 true，此时会采集每个进程的资源占用情况，并附上 pid 作为标签来区分

## 告警规则

夜莺内置了进程监控的告警规则，克隆到自己的业务组下即可使用。

## 仪表盘

夜莺内置了进程监控的仪表盘，克隆到自己的业务组下即可使用。
