# Logstash

Logstash exposes node, JVM, and pipeline statistics through its HTTP API,
usually at `http://127.0.0.1:9600`. Categraf's Logstash input collects this API.

## Configuration

```bash
curl -fsS http://127.0.0.1:9600/_node/stats | head
```

Configure `conf/input.logstash/logstash.toml`:

```toml
[[instances]]
url = "http://127.0.0.1:9600"
collect = ["pipelines", "process", "jvm"]
timeout = "5s"
labels = { instance = "logstash-01" }
```

Configure Basic Auth and TLS when enabled. Event-rate, queue, and pipeline
panels require Logstash to process real events.

See the [Categraf example](https://github.com/flashcatcloud/categraf/blob/main/conf/input.logstash/logstash.toml).
