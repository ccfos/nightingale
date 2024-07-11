# HAProxy

forked from [haproxy_exporter](https://github.com/prometheus/haproxy_exporter)

Note: since HAProxy 2.0.0, the official source includes a Prometheus exporter module that can be built into your binary with a single flag during build time and offers an exporter-free Prometheus endpoint.


haproxy configurations for `/stats`:

```
frontend stats
    bind *:8404
    stats enable
    stats uri /stats
    stats refresh 10s
```