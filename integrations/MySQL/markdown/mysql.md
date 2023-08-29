# mysql

mysql 监控采集插件，核心原理就是连到 mysql 实例，执行一些 sql，解析输出内容，整理为监控数据上报。

## Configuration

```toml
# # collect interval
# interval = 15

# 要监控 MySQL，首先要给出要监控的MySQL的连接地址、用户名、密码
[[instances]]
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

当主机填写为localhost时mysql会采用 unix domain socket连接
当主机填写为127.0.0.1时mysql会采用tcp方式连接
大家最常问的问题是如何监控多个mysql实例，实际大家对toml配置学习一下就了解了，`[[instances]]` 部分表示数组，是可以出现多个的，address参数支持通过unix路径连接 所以，举例：

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

## 监控大盘和告警规则

本 README 的同级目录，大家可以看到alerts.json 是告警规则，导入夜莺就可以使用， dashboard-by-instance.json 就是监控大盘（注意！监控大盘使用instance大盘变量，所以，上面的配置文件中要配置一个instance的标签，就是 `labels = { instance="n9e-10.2.3.4:3306" }` 部分），也是导入夜莺就可以使用。dashboard-by-ident是使用ident作为大盘变量，适用于先找到宿主机器，再找机器上面的mysql实例的场景