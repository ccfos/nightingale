# Oracle plugin

Oracle 插件，用于监控 Oracle 数据库。默认无法跑在 Windows 上。如果你的 Oracle 部署在 Windows 上，也没问题，使用部署在 Linux 上的 Categraf 远程监控 Windows 上的 Oracle，也行得通。

Oracle 插件的核心监控原理，就是执行下面 [这些 SQL 语句](https://github.com/flashcatcloud/categraf/blob/main/conf/input.oracle/metric.toml)，然后解析出结果，上报到监控服务端。

以其中一个为例：

```toml
[[metrics]]
mesurement = "activity"
metric_fields = [ "value" ]
field_to_append = "name"
timeout = "3s"
request = '''
SELECT name, value FROM v$sysstat WHERE name IN ('parse count (total)', 'execute count', 'user commits', 'user rollbacks')
'''
```

- mesurement：指标类别
- label_fields：作为 label 的字段
- metric_fields：作为 metric 的字段，因为是作为 metric 的字段，所以这个字段的值必须是数字
- field_to_append：表示这个字段附加到 metric_name 后面，作为 metric_name 的一部分
- timeout：超时时间
- request：具体查询的 SQL 语句

如果你想监控的指标，默认没有采集，只需要增加自定义的 `[[metrics]]` 配置即可。
