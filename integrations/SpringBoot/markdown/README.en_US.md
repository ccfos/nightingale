# SpringBoot

For projects in the Java ecosystem, micrometer is a common choice for exposing metrics data. However, SpringBoot projects can directly use SpringBoot Actuator to expose metrics data. Actuator is also implemented on top of micrometer under the hood — it is just simpler to use.

Add `spring-boot-starter-actuator` and `micrometer-registry-prometheus`.

See [Spring Boot Metrics](https://docs.spring.io/spring-boot/reference/actuator/metrics.html).

## Application configuration

For Spring Boot 3.x:

```properties
management.endpoints.web.exposure.include=health,info,prometheus
management.endpoint.prometheus.enabled=true
management.prometheus.metrics.export.enabled=true
```

For Spring Boot 2.x:

```properties
management.endpoints.web.exposure.include=health,info,prometheus
management.endpoint.prometheus.enabled=true
management.metrics.export.prometheus.enabled=true
```

Avoid exposing every Actuator endpoint unless it is required.

## Collection configuration

Create `conf/input.prometheus/springboot.toml`:

```toml
[[instances]]
urls = ["http://127.0.0.1:8080/actuator/prometheus"]
url_label_key = "instance"
url_label_value = "{{.Host}}"
labels = { job = "springboot", application = "order-service" }
```

Keep `application` stable across replicas and `instance` unique. Generate real
HTTP traffic and GC activity before validating rate and latency panels.
