# 进程总量统计

如果进程总量太多，比如超过了 CPU core 的 3 倍，就需要关注了。

## 配置说明

configuration file: `conf/input.processes/processes.toml`

默认配置如下（一般维持默认不用动）：

```toml
# # collect interval
# interval = 15

# # force use ps command to gather
# force_ps = false

# # force use /proc to gather
# force_proc = false
```

有两种采集方式，使用 ps 命令，或者直接读取 `/proc` 目录，默认是后者。如果想强制使用 ps 命令才采集，开启 force_ps 即可：

```toml
force_ps = true
```

## 仪表盘

夜莺内置了进程总量的仪表盘，克隆到自己的业务组下即可使用。

## 告警规则

夜莺内置了进程总量的告警规则，克隆到自己的业务组下即可使用。

