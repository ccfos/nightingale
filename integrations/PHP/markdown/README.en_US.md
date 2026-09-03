# PHP-FPM

*PHP-FPM* (PHP FastCGI Process Manager) monitoring collection plugin, adapted from telegraf's phpfpm plugin.

This plugin requires changing the phpfpm configuration file to enable the *pm.status_path* option:
```
pm.status_path = /status
```


## Configuration

Please refer to the sample [configuration](https://github.com/flashcatcloud/categraf/blob/main/conf/input.phpfpm/phpfpm.toml) file.

### Notes:
1. The following options only take effect for HTTP URLs:
    - response_timeout
    - username & password
    - headers
    - TLS config
2. If you use a Unix socket, make sure categraf and the socket path are on the same host, and that the user running categraf has read permission on that path.
## Dashboards and Alert Rules

To be updated...
