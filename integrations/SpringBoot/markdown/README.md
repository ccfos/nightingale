# SpringBoot

Java 生态的项目，如果要暴露 metrics 数据，一般可以选择 micrometer，不过 SpringBoot 项目可以直接使用 SpringBoot Actuator 暴露 metrics 数据，Actuator 底层也是使用 micrometer 来实现的，只是使用起来更加简单。

项目需要引入 `spring-boot-starter-actuator` 和 `micrometer-registry-prometheus`。

参考：[Spring Boot Metrics](https://docs.spring.io/spring-boot/reference/actuator/metrics.html)。

## 应用配置

Spring Boot 3.x 在 `application.properties` 中加入：

```properties
management.endpoints.web.exposure.include=health,info,prometheus
management.endpoint.prometheus.enabled=true
management.prometheus.metrics.export.enabled=true
```

Spring Boot 2.x 使用：

```properties
management.endpoints.web.exposure.include=health,info,prometheus
management.endpoint.prometheus.enabled=true
management.metrics.export.prometheus.enabled=true
```

启动后确认 `http://localhost:8080/actuator/prometheus` 能返回指标。除非确有需要，不建议把 `management.endpoints.web.exposure.include` 设置成 `*`。

## 采集配置

新建 `conf/input.prometheus/springboot.toml`：

```toml
[[instances]]
urls = ["http://127.0.0.1:8080/actuator/prometheus"]
url_label_key = "instance"
url_label_value = "{{.Host}}"
labels = { job = "springboot", application = "order-service" }
```

模板使用 `application`、`job` 或 `instance` 筛选应用。多实例部署时保持 `application` 相同、`instance` 唯一。只有产生真实 HTTP 请求、线程活动和 GC 后，对应速率及延迟面板才会出现数据。
