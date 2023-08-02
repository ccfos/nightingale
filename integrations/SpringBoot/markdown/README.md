# SpringBoot

Java 生态的项目，如果要暴露 metrics 数据，一般可以选择 micrometer，不过 SpringBoot 项目可以直接使用 SpringBoot Actuator 暴露 metrics 数据，Actuator 底层也是使用 micrometer 来实现的，只是使用起来更加简单。

## 应用配置

在 application.properties 中加入如下配置：

```properties
management.endpoint.metrics.enabled=true
management.endpoints.web.exposure.include=*
management.endpoint.prometheus.enabled=true
management.metrics.export.prometheus.enabled=true
```

完事启动项目，访问 `http://localhost:8080/actuator/prometheus` 即可看到符合 prometheus 协议的监控数据。

## 采集配置

既然暴露了 Prometheus 协议的监控数据，那通过 categraf prometheus 插件直接采集即可。配置文件是 `conf/input.prometheus/prometheus.toml`。配置样例如下：

```toml
[[instances]]
urls = [
     "http://192.168.11.177:8080/actuator/prometheus"
]
```

## 仪表盘

夜莺内置了一个 SpringBoot 仪表盘，由网友贡献，克隆到自己的业务组下即可使用，欢迎大家一起来提 PR 完善。

![actuator2.0](http://download.flashcat.cloud/uPic/actuator_2.0.png)
