# N9E

Nightingale V5's n9e-webapi and n9e-server, and the V6–V9 n9e process,
expose Prometheus metrics at `/metrics`. n9e-edge exposes the same endpoint.
The real-data validation used Nightingale V9.

Therefore, Nightingale's monitoring data can be collected simply with categraf's prometheus plugin.

## Collector configuration

Create `conf/input.prometheus/n9e.toml`:

```toml
[[instances]]
urls = ["http://127.0.0.1:17000/metrics"]
url_label_key = "instance"
url_label_value = "{{.Host}}"
labels = { job = "n9e" }
```

Scrape every n9e/n9e-edge instance and keep `instance` unique. Verify with
`curl -fsS http://127.0.0.1:17000/metrics | head`.
