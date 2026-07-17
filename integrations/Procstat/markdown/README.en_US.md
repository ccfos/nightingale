# Process monitoring

Uses the categraf procstat plugin.

## Configuration file

Location: `conf/input.procstat/procstat.toml` under the categraf directory.

Sample configuration:

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

A machine runs many processes. To monitor whether a process is alive and how much resource it consumes, you first have to tell categraf which processes to monitor. That is why the first few settings of this plugin are for process filtering — they tell categraf which processes to watch.

- search_exec_substring configures a search string, equivalent to running `pgrep <search_exec_substring>`
- search_cmdline_substring configures a search string, equivalent to running `pgrep -f <search_cmdline_substring>`
- search_win_service configures a Windows service name, equivalent to running `sc query <search_win_service>`

The example above collects nginx by default. By default only one metric is collected: procstat_lookup_count, which is the number of processes matched by these filters. Obviously, if `procstat_lookup_count <= 0`, the process no longer exists.

## CPU usage calculation

There are two modes for calculating CPU usage: irix (default) and solaris. In irix mode, CPU usage can exceed 100%; in solaris mode, the number of CPU cores is taken into account, so CPU usage never exceeds 100%.

## Collecting more metrics

`gather_more_metrics` is not enabled by default, i.e. process resource usage is not collected. To collect it, simply enable the `gather_more_metrics` setting. The most special entry is `jvm`: to collect jvm metrics, install jstat first, then enable the `jvm` entry.

## gather_total

For example, multiple processes named mysql may be running at the same time. If we want to know the total cpu, mem, fd, etc. consumed by all mysql processes on the machine, set gather_total = true. Note that for uptime and limit, gather_total takes the minimum value across the processes.

## gather_per_pid

Again taking mysql as an example: multiple instances may run on one machine, and we may want to know the resource usage of each mysql process. In that case, enable the gather_per_pid setting by setting it to true. The resource usage of each process will then be collected, with the pid attached as a label to distinguish them.
