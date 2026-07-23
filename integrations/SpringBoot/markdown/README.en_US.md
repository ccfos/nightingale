# SpringBoot

For projects in the Java ecosystem, micrometer is a common choice for exposing metrics data. However, SpringBoot projects can directly use SpringBoot Actuator to expose metrics data. Actuator is also implemented on top of micrometer under the hood — it is just simpler to use.

## Application configuration

Add the following configuration to application.properties:

```properties
management.endpoint.metrics.enabled=true
management.endpoints.web.exposure.include=*
management.endpoint.prometheus.enabled=true
management.metrics.export.prometheus.enabled=true
```

Then start the project and visit `http://localhost:8080/actuator/prometheus` to see monitoring data in prometheus format.

## Collection configuration

Since the monitoring data is exposed in Prometheus format, it can be collected directly with the categraf prometheus plugin. The configuration file is `conf/input.prometheus/prometheus.toml`. A sample configuration:

```toml
[[instances]]
urls = [
     "http://192.168.11.177:8080/actuator/prometheus"
]
```
