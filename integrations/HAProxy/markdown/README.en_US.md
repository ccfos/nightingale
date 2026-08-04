# HAProxy

This template has been verified against real HAProxy backend traffic using Categraf's HAProxy input.
The input can read either the HAProxy CSV stats page or the Runtime API socket.

## Enable HAProxy stats

```haproxy
frontend stats
    bind *:8404
    stats enable
    stats uri /stats
    stats refresh 10s
```

First make sure the CSV output is reachable:

```bash
curl -fsS 'http://127.0.0.1:8404/stats;csv' | head
```

## Categraf configuration

The configuration file is `conf/input.haproxy/haproxy.toml`:

```toml
[[instances]]
uri = "http://127.0.0.1:8404/stats;csv"
ssl_verify = true
timeout = "5s"
```

A socket can be used as well:

```toml
[[instances]]
uri = "unix:/run/haproxy/admin.sock"
timeout = "5s"
```

HAProxy 2.0 and above can also be built with the native Prometheus exporter and scraped by Categraf's
Prometheus input, but the `HAProxy By Categraf` template in this directory is verified against the metric
names of the Categraf HAProxy input. Keep sending real requests to the frontend/backend, otherwise the
rate and response-code panels may stay empty.
