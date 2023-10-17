# prometheus

prometheus 插件的作用，就是抓取 `/metrics` 接口的数据，上报给服务端。通过，各类 exporter 会暴露 `/metrics` 接口数据，越来越多的开源组件也会内置 prometheus SDK，吐出 prometheus 格式的监控数据，比如 rabbitmq 插件，其 README 中就有介绍。

这个插件 fork 自 telegraf/prometheus，做了一些删减改造，仍然支持通过 consul 做服务发现，管理所有的目标地址，删掉了 Kubernetes 部分，Kubernetes 部分准备放到其他插件里实现。

增加了两个配置：url_label_key 和 url_label_value。为了标识监控数据是从哪个 scrape url 拉取的，会为监控数据附一个标签来标识这个 url，默认的标签 KEY 是用 instance，当然，也可以改成别的，不过不建议。url_label_value 是标签值，支持 go template 语法，如果为空，就是整个 url 的内容，也可以通过模板变量只取一部分，比如 `http://localhost:9104/metrics`，只想取 IP 和端口部分，就可以写成：

```ini
url_label_value = "{{.Host}}"
```

如果 HTTP scheme 部分和 `/metrics` Path 部分都想取，可以这么写：

```ini
url_label_value = "{{.Scheme}}://{{.Host}}{{.Path}}"
```

相关变量是用这个方法生成的，供大家参考：

```go
func (ul *UrlLabel) GenerateLabel(u *url.URL) (string, string, error) {
	if ul.LabelValue == "" {
		return ul.LabelKey, u.String(), nil
	}

	dict := map[string]string{
		"Scheme":   u.Scheme,
		"Host":     u.Host,
		"Hostname": u.Hostname(),
		"Port":     u.Port(),
		"Path":     u.Path,
		"Query":    u.RawQuery,
		"Fragment": u.Fragment,
	}

	var buffer bytes.Buffer
	err := ul.LabelValueTpl.Execute(&buffer, dict)
	if err != nil {
		return "", "", err
	}

	return ul.LabelKey, buffer.String(), nil
}
```