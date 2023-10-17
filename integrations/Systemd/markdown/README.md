# systemd 插件
自 [node_exporter](https://github.com/prometheus/node_exporter/blob/master/collector/systemd_linux.go) fork 并改动

## Configuration
```toml
enable=false # 设置为true 打开采集
#unit_include=".+"
#unit_exclude=""
enable_start_time_metrics=true #是否采集service unit的启动时间信息 单位秒
enable_task_metrics=true # 是否采集service unit task的metrics
enable_restarts_metrics=true #是否采集service unit重启的次数信息
```
