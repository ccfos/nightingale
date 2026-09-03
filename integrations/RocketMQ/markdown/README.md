
配置示例
```
#[mappings]
# add labels based on source ip
#"172.18.0.7" = { "app" = "bar" }
#"172.18.0.3" = { "app" = "foo" }

#add labels based on headers
#"broker:am" = {"my-cluster"="node-am"}
#"broker:bm" = {"my-cluster"="node-bm"}

#[[instances]]
#endpoint="0.0.0.0:4317"
#cert_file="/path/to/server.crt"
#key_file="/path/to/server.key"
```