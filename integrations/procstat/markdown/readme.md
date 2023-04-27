## Categraf as collector

configuration file: `conf/input.procstat/procstat.toml`

进程监控插件，两个核心作用，监控进程是否存活、监控进程使用了多少资源（CPU、内存、文件句柄等）

### 存活监控

如果进程监听了端口，就直接用 net_response 来做存活性监控即可，无需使用 procstat 来做，因为：端口在监听，说明进程一定活着，反之则不一定。

### 进程筛选

机器上进程很多，我们要做进程监控，就要想办法告诉 Categraf 要监控哪些进程，通过 search 打头的那几个配置，可以做进程过滤筛选：

```toml
[[instnaces]]
# # executable name (ie, pgrep <search_exec_substring>)
search_exec_substring = "nginx"

# # pattern as argument for pgrep (ie, pgrep -f <search_cmdline_substring>)
# search_cmdline_substring = "n9e server"

# # windows service name
# search_win_service = ""
```

上面三个 search 相关的配置，每个采集目标选用其中一个。有一个额外的配置：search_user，配合search_exec_substring 或者 search_cmdline_substring 使用，表示匹配指定 username 的特定进程。如果不需要指定username，保持配置注释即可。

```toml
# # search process with specific user, option with exec_substring or cmdline_substring
# search_user = ""
```

默认的进程监控的配置，`[[instnaces]]` 是注释掉的，记得打开。

### mode

mode 配置有两个值供选择，一个是 solaris，一个是 irix，默认是 irix，用这个配置来决定使用哪种 cpu 使用率的计算方法：

```go
func (ins *Instance) gatherCPU(slist *types.SampleList, procs map[PID]Process, tags map[string]string, solarisMode bool) {
	var value float64
	for pid := range procs {
		v, err := procs[pid].Percent(time.Duration(0))
		if err == nil {
			if solarisMode {
				value += v / float64(runtime.NumCPU())
				slist.PushFront(types.NewSample("cpu_usage", v/float64(runtime.NumCPU()), map[string]string{"pid": fmt.Sprint(pid)}, tags))
			} else {
				value += v
				slist.PushFront(types.NewSample("cpu_usage", v, map[string]string{"pid": fmt.Sprint(pid)}, tags))
			}
		}
	}

	if ins.GatherTotal {
		slist.PushFront(types.NewSample("cpu_usage_total", value, tags))
	}
}
```

### gather_total

比如进程名字是 mysql 的进程，同时可能运行了多个，我们想知道这个机器上的所有 mysql 的进程占用的总的 cpu、mem、fd 等，就设置 gather_total = true，当然，对于 uptime 和 limit 的采集，gather_total 的时候是取的多个进程的最小值

### gather_per_pid

还是拿 mysql 举例，一个机器上可能同时运行了多个，我们可能想知道每个 mysql 进程的资源占用情况，此时就要启用 gather_per_pid 的配置，设置为 true，此时会采集每个进程的资源占用情况，并附上 pid 作为标签来区分

### gather_more_metrics

默认 procstat 插件只是采集进程数量，如果想采集进程占用的资源，就要启用 gather_more_metrics 中的项，启用哪个就额外采集哪个

### jvm

gather_more_metrics 中有个 jvm，如果是 Java 的进程可以选择开启，非 Java 的进程就不要开启了。需要注意的是，这个监控需要依赖机器上的 jstat 命令，这是社区小伙伴贡献的采集代码，感谢 [@lsy1990](https://github.com/lsy1990)

### One more thing

要监控什么进程就去目标机器修改 Categraf 的配置 `conf/input.procstat/procstat.toml` ，如果嫌麻烦，可以联系我们采购专业版，专业版支持在服务端 WEB 上统一做配置，不需要登录目标机器修改 Categraf 的配置。
