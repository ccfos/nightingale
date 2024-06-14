# mysql

mysql 监控采集插件，核心原理就是连到 mysql 实例，执行一些 sql，解析输出内容，整理为监控数据上报。

## Configuration

categraf 的 `conf/input.mysql/mysql.toml`

```toml
[[instances]]
# 要监控 MySQL，首先要给出要监控的MySQL的连接地址、用户名、密码
address = "127.0.0.1:3306"
username = "root"
password = "1234"

# # set tls=custom to enable tls
# parameters = "tls=false"

# 通过 show global status监控mysql，默认抓取一些基础指标，
# 如果想抓取更多global status的指标，把下面的配置设置为true
extra_status_metrics = true

# 通过show global variables监控mysql的全局变量，默认抓取一些常规的
# 常规的基本够用了，扩展的部分，默认不采集，下面的配置设置为false
extra_innodb_metrics = false

# 监控processlist，关注较少，默认不采集
gather_processlist_processes_by_state = false
gather_processlist_processes_by_user = false

# 监控各个数据库的磁盘占用大小
gather_schema_size = false

# 监控所有的table的磁盘占用大小
gather_table_size = false

# 是否采集系统表的大小，通过不用，所以默认设置为false
gather_system_table_size = false

# 通过 show slave status监控slave的情况，比较关键，所以默认采集
gather_slave_status = true

# # timeout
# timeout_seconds = 3

# # interval = global.interval * interval_times
# interval_times = 1

# 为mysql实例附一个instance的标签，因为通过address=127.0.0.1:3306不好区分
# important! use global unique string to specify instance
# labels = { instance="n9e-10.2.3.4:3306" }

## Optional TLS Config
# use_tls = false
# tls_min_version = "1.2"
# tls_ca = "/etc/categraf/ca.pem"
# tls_cert = "/etc/categraf/cert.pem"
# tls_key = "/etc/categraf/key.pem"
## Use TLS but skip chain & host verification
# insecure_skip_verify = true

# 自定义SQL，指定SQL、返回的各个列那些是作为metric，哪些是作为label
# [[instances.queries]]
# mesurement = "users"
# metric_fields = [ "total" ]
# label_fields = [ "service" ]
# # field_to_append = ""
# timeout = "3s"
# request = '''
# select 'n9e' as service, count(*) as total from n9e_v5.users
# '''
```

## 监控多个实例

大家最常问的问题是如何监控多个mysql实例，实际大家对toml配置学习一下就了解了，`[[instances]]` 部分表示数组，是可以出现多个的，举例：

```toml
[[instances]]
address = "10.2.3.6:3306"
username = "root"
password = "1234"
labels = { instance="n9e-10.2.3.6:3306" }

[[instances]]
address = "10.2.6.9:3306"
username = "root"
password = "1234"
labels = { instance="zbx-10.2.6.9:3306" }

[[instances]]
address = "/tmp/mysql.sock"
username = "root"
password = "1234"
labels = { instance="zbx-localhost:3306" }
```

## 监控大盘

夜莺内置了 mysql 相关的监控大盘，内置了至少 4 个仪表盘：

### mysql_by_categraf_instance

这个大盘是使用 categraf 作为采集器，使用 instance 作为大盘变量，所以上例采集配置中都有一个 instance 的标签，就是和这个大盘配合使用的。

### mysql_by_categraf_ident

这个大盘是使用 categraf 作为采集器，使用 ident 作为大盘变量，即在查看 mysql 监控指标的时候，先通过大盘选中宿主机器，再通过机器找到 mysql 实例。

### dashboard-by-aws-rds

这是网友贡献的大盘，采集的 aws 的 rds 相关的数据制作的大盘。欢迎各位网友贡献大盘，这是一个很好的共建社区的方式。把您做好的大盘导出为 JSON，提 PR 到 [这个目录](https://github.com/ccfos/nightingale/tree/main/integrations/MySQL/dashboards) 下即可。

### mysql_by_exporter

这是使用 mysqld_exporter 作为采集器制作的大盘。

## 告警规则

夜莺内置了 mysql 相关的告警规则，克隆到自己的业务组即可使用。也欢迎大家一起来通过 PR 完善修改这个内置的 [告警规则](https://github.com/ccfos/nightingale/tree/main/integrations/MySQL/alerts)。