# mysql

MySQL monitoring collection plugin. The core idea is simple: connect to the MySQL instance, run some SQL statements, parse the output, and turn it into monitoring data to report.

## Configuration

categraf's `conf/input.mysql/mysql.toml`

```toml
[[instances]]
# To monitor MySQL, first provide the connection address, username, and password
# of the MySQL instance to be monitored
address = "127.0.0.1:3306"
username = "root"
password = "1234"

# # set tls=custom to enable tls
# parameters = "tls=false"

# Monitor MySQL via `show global status`; a set of basic metrics is collected by default.
# If you want to collect more global status metrics, set the option below to true
extra_status_metrics = true

# Monitor MySQL global variables via `show global variables`; the common ones are
# collected by default, which is usually enough. The extended set is not collected
# by default, so the option below is set to false
extra_innodb_metrics = false

# Monitor the processlist; rarely needed, so not collected by default
gather_processlist_processes_by_state = false
gather_processlist_processes_by_user = false

# Monitor the disk usage of each database
gather_schema_size = false

# Monitor the disk usage of all tables
gather_table_size = false

# Whether to collect the size of system tables; generally not needed, so false by default
gather_system_table_size = false

# Monitor replica status via `show slave status`; this is critical, so collected by default
gather_slave_status = true

# # timeout
# timeout_seconds = 3

# # interval = global.interval * interval_times
# interval_times = 1

# Attach an instance label to the MySQL instance, since address=127.0.0.1:3306
# is hard to distinguish between hosts
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

# Custom SQL: specify the SQL statement and which returned columns are used
# as metrics and which as labels
# [[instances.queries]]
# measurement = "users"
# metric_fields = [ "total" ]
# label_fields = [ "service" ]
# # field_to_append = ""
# timeout = "3s"
# request = '''
# select 'n9e' as service, count(*) as total from n9e_v5.users
# '''
```

## Monitoring Multiple Instances

The most frequently asked question is how to monitor multiple MySQL instances. Once you learn a bit about the TOML format, it becomes obvious: `[[instances]]` denotes an array element, so it can appear multiple times. For example:

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
