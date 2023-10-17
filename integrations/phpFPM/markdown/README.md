# PHP-FPM

*PHP-FPM* (PHP FastCGI Process Manager) 监控采集插件，由telegraf的phpfpm改造而来。

该插件需要更改phpfpm的配置文件，开启 *pm.status_path*配置项
```
pm.status_path = /status
```


## Configuration

请参考配置[示例](https://github.com/flashcatcloud/categraf/blob/main/conf/input.phpfpm/phpfpm.toml)文件

### 注意事项：
1. 如下配置 仅生效于HTTP的url
    - response_timeout
    - username & password
    - headers
    - TLS config
2. 如果使用 Unix socket，需要保证 categraf 和 socket path 在同一个主机上，且 categraf 运行用户拥有读取该 path 的权限。
## 监控大盘和告警规则

待更新...