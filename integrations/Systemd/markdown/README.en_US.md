# systemd Plugin
The Systemd plugin monitors the status and performance metrics of systemd services on Linux systems. It communicates with systemd over the D-Bus interface and collects key monitoring data such as service running states, start times, task counts, and restart counts.

## System Requirements
 - Operating system: Linux
 - systemd version: 212 or later recommended (some features require newer versions)
 - Permissions: access to the systemd D-Bus interface is required

## Configuration
```toml
# Whether to enable the systemd plugin
enable = true

# Regular expression for units to include, defaults to ".+" (all units)
unit_include = '''.+'''

# Regular expression for units to exclude, by default excludes automount, device, mount, scope, and slice types
unit_exclude = '''.+\.(automount|device|mount|scope|slice)'''

# Whether to collect start time metrics for service units (unit: seconds)
enable_start_time_metrics = true

# Whether to collect task metrics for service units
enable_task_metrics = true

# Whether to collect restart count metrics for service units
enable_restarts_metrics = true

# Whether to use a private systemd connection
systemd_private = false
```
## Configuration Parameters
|Parameter|Type|Default|Description|
|-|-|-|-|
|enable|bool|false|Whether to enable the plugin|
|unit_include|string|.+|Regular expression for unit names to include|
|unit_exclude|string|	.+\\.(automount\|device\|mount\|scope\|slice)|Regular expression for unit names to exclude|
|enable_start_time_metrics|bool|true|Whether to collect service start time metrics|
|enable_task_metrics|bool|true|Whether to collect task-related metrics|
|enable_restarts_metrics|bool|true|Whether to collect restart count metrics|
|systemd_private|bool|false|Whether to use a private systemd connection|

## Metrics
### System-level Metrics
|Name|Type|Labels|Description|
|-|-|-|-|
|systemd_version|gauge|version|systemd version information|
|systemd_units|gauge|state|Total number of units in each state|
|systemd_system_running|gauge|-|Whether the system is in the running state (1 = running, 0 = not running)|
### Unit State Metrics
|Name|Type|Labels|Description|
|-|-|-|-|
|systemd_unit_state|gauge|name, state, type|Unit state information|
|systemd_unit_start_time_seconds|gauge|name|Unit start timestamp (Unix timestamp)|
### Service Metrics
|Name|Type|Labels|Description|
|-|-|-|-|
|systemd_service_restart_total|counter|name|Total number of service restarts|
|systemd_unit_tasks_current|gauge|name|Current number of tasks|
|systemd_unit_tasks_max|gauge|name|Maximum task limit|
### Socket Metrics
|Name|Type|Labels|Description|
|-|-|-|-|
|systemd_socket_accepted_connections_total|counter|name|Total number of connections accepted by the socket|
|systemd_socket_current_connections|gauge|name|Current number of socket connections|
|systemd_socket_refused_connections_total|counter|name|Total number of connections refused by the socket|
### Timer Metrics
|Name|Type|Labels|Description|
|-|-|-|-|
|systemd_timer_last_trigger_seconds|gauge|name|Timestamp of the timer's last trigger|

## Best Practices
1. Configure filtering rules sensibly. Set appropriate include and exclude rules based on your monitoring needs to avoid collecting too much useless data:
```toml
# Example: only monitor application services, exclude internal system services
unit_include = '''^(app-|web-|db-).*\.service$'''
unit_exclude = '''^(systemd|dbus|udev).*'''
```
2. Enable features on demand. Selectively enable feature modules according to actual needs:
```toml
# For most application monitoring scenarios
enable_start_time_metrics = true
enable_task_metrics = false      # can be disabled if you don't care about task counts
enable_restarts_metrics = true
```
3. Alerting configuration. It is recommended to configure alerts for the following metrics:
- Abnormal service state: systemd_unit_state{state!="active"} == 1
- Frequent service restarts: increase(systemd_service_restart_total[5m]) > 3
- Abnormal system state: systemd_system_running == 0

## Troubleshooting
1. Insufficient permissions
- Make sure the categraf process has permission to access the systemd D-Bus interface
- Check that the systemd service is running properly
2. Version compatibility
- Some metrics require systemd 212 or later
- Check the systemd version on your system: systemctl --version
3. D-Bus connection issues
- Check the D-Bus service status: systemctl status dbus
- Try setting systemd_private = true in the configuration file conf/input.systemd/systemd.toml
