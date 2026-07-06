# systemd 插件
Systemd 插件用于监控 Linux 系统中 systemd 服务的状态和性能指标。该插件通过 D-Bus 接口与 systemd 通信，收集系统服务的运行状态、启动时间、任务数量、重启次数等关键监控数据。

## 系统要求
 - 操作系统：Linux
 - systemd 版本：建议 212 及以上版本（部分功能需要更高版本支持）
 - 权限：需要访问 systemd D-Bus 接口的权限

## Configuration
```toml
# 是否启用 systemd 插件
enable = true

# 包含的 unit 正则表达式，默认为 ".+"（所有unit）
unit_include = '''.+'''

# 排除的 unit 正则表达式，默认排除 automount、device、mount、scope、slice 类型
unit_exclude = '''.+\.(automount|device|mount|scope|slice)'''

# 是否采集 service unit 的启动时间信息（单位：秒）
enable_start_time_metrics = true

# 是否采集 service unit task 的 metrics
enable_task_metrics = true

# 是否采集 service unit 重启次数信息
enable_restarts_metrics = true

# 是否使用私有 systemd 连接
systemd_private = false
```
## 配置参数说明
|参数|类型|默认值|说明|
|-|-|-|-|
|enable|bool|false|是否启用插件|
|unit_include|string|.+|包含的 unit 名称正则表达式|
|unit_exclude|string|	.+\\.(automount\|device\|mount\|scope\|slice)|排除的 unit 名称正则表达式|
|enable_start_time_metrics|bool|true|是否收集服务启动时间指标|
|enable_task_metrics|bool|true|是否收集任务相关指标|
|enable_restarts_metrics|bool|true|是否收集重启次数指标|
|systemd_private|bool|false|是否使用私有 systemd 连接|

## 监控指标
### 系统级指标
|名称|类型|标签|说明|
|-|-|-|-|
|systemd_version|gauge|version|systemd 版本信息|
|systemd_units|gauge|state|各状态下的 unit 总数|
|systemd_system_running|gauge|-|系统是否处于运行状态（1=运行，0=非运行）|
### Unit状态指标
|名称|类型|标签|说明|
|-|-|-|-|
|systemd_unit_state|gauge|name, state, type|unit 的状态信息|
|systemd_unit_start_time_seconds|gauge|name|unit 启动时间戳（Unix时间戳）|
### 服务相关指标
|名称|类型|标签|说明|
|-|-|-|-|
|systemd_service_restart_total|counter|name|服务重启总次数|
|systemd_unit_tasks_current|gauge|name|当前任务数量|
|systemd_unit_tasks_max|gauge|name|最大任务数量限制|
### Socket相关指标
|名称|类型|标签|说明|
|-|-|-|-|
|systemd_socket_accepted_connections_total|counter|name|socket 接受的连接总数|
|systemd_socket_current_connections|gauge|name|socket 当前连接数|
|systemd_socket_refused_connections_total|counter|name|socket 拒绝的连接总数|
### Timer相关指标
|名称|类型|标签|说明|
|-|-|-|-|
|systemd_timer_last_trigger_seconds|gauge|name|timer 上次触发时间戳|

## 最佳实践
1. 合理配置过滤规则 根据监控需求设置合适的包含和排除规则，避免收集过多无用数据：
```toml
# 示例：只监控应用服务，排除系统内部服务
unit_include = '''^(app-|web-|db-).*\.service$'''
unit_exclude = '''^(systemd|dbus|udev).*'''
```
2. 按需启用功能 根据实际需求选择性启用功能模块：
```toml
# 对于大多数应用监控场景
enable_start_time_metrics = true
enable_task_metrics = false      # 如果不关心任务数量可关闭
enable_restarts_metrics = true
```
3. 监控告警配置 建议为以下指标配置告警：
- 服务状态异常：systemd_unit_state{state!="active"} == 1
- 服务频繁重启：increase(systemd_service_restart_total[5m]) > 3
- 系统状态异常：systemd_system_running == 0

## 故障排查
1. 权限不足
- 确保 categraf 进程有权限访问 systemd D-Bus 接口
- 检查 systemd 服务是否正常运行
2. 版本兼容性
- 某些指标需要 systemd 212+ 版本支持
- 检查系统 systemd 版本：systemctl –version
3. D-Bus 连接问题
- 检查 D-Bus 服务状态：systemctl status dbus
- 配置文件conf/input.systemd/systemd.toml中尝试设置 systemd_private = true