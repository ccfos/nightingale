# TDEngine

TDEngine can also expose monitoring data in Prometheus format. How to enable it:

TODO

## Collection configuration

Since the monitoring data is exposed in Prometheus format, it can be collected directly with the categraf prometheus plugin. The configuration file is `conf/input.prometheus/prometheus.toml`. A sample configuration:

```toml
[[instances]]
urls = [
     "http://192.168.11.177:8080/xxxx"
]
```
