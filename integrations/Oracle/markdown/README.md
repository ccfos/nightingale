# Oracle plugin

Oracle 插件，用于监控 Oracle 数据库。下载 Categraf 的时候，使用 `--with-cgo` 包名的二进制。目前只提供 Linux 版本的二进制，默认无法跑在 Windows 上。如果你的 Oracle 部署在 Windows 上，也没问题，使用部署在 Linux 上的 Categraf 远程监控 Windows 上的 Oracle，也行得通。

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

如果你想监控的指标，默认没有采集，只需要修改 [metric.toml](https://github.com/flashcatcloud/categraf/blob/main/conf/input.oracle/metric.toml)，增加自己的采集 SQL 即可。

## 仪表盘

夜莺内置了 Oracle 仪表盘，克隆到自己的业务组下即可使用。

## 技术支持

上面的文档认真理解并实验，理论上就懂得如何使用了。如果还是不懂，可以在 [论坛](https://answer.flashcat.cloud/) 寻求技术支持，不过 Oracle 插件比较复杂，我们只为社区贡献者（比如提过 PR、写过夜莺相关的博客）和商业用户提供技术支持（精力着实有限，顾不过来），望理解。
