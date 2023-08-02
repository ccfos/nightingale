# Windows

categraf 不但支持 linux 监控数据采集，也支持 windows 监控数据采集，而且指标命名也是一样的，这样告警规则、仪表盘其实都可以复用。不需要对 windows 做额外处理。

## 安装

categraf 在 windows 下安装请参考这个 [文档](https://flashcat.cloud/docs/content/flashcat-monitor/categraf/2-installation/)。

## 仪表盘

linux、windows 仪表盘其实是可以复用的，只是两种操作系统个别指标不同。比如有些指标是 linux 特有的，有些指标是 windows 特有的。如果你想要分开查看，夜莺也内置了 windows 的仪表盘，克隆到自己的业务组下即可使用。

## 告警规则

夜莺虽然也内置了 windows 的告警规则，但因为 linux、windows 大部分指标都是一样的，就不建议为 windows 单独管理一份告警规则了。
