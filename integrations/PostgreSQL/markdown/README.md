## PostgreSQL Dashboard & Alerts

使用[categraf](https://github.com/flashcatcloud/categraf)中[inputs.postgresql](https://github.com/flashcatcloud/categraf/tree/main/inputs/postgresql)插件采集[PostgreSQL](https://www.postgresql.org/)服务监控指标数据；

### 配置文件示例：

如果是多个PostgreSQL，可以写多个[[instance]]配置

```toml
[[instances]]
address = "host=192.168.11.181 port=5432 user=postgres password=123456789 sslmode=disable"
## specify address via a url matching:
##   postgres://[pqgotest[:password]]@localhost[/dbname]?sslmode=[disable|verify-ca|verify-full]
## or a simple string:
##   host=localhost user=pqgotest password=... sslmode=... dbname=app_production
##
## All connection parameters are optional.
##
## Without the dbname parameter, the driver will default to a database
## with the same name as the user. This dbname is just for instantiating a
## connection with the server and doesn't restrict the databases we are trying
## to grab metrics for.
##
# address = "host=localhost user=postgres sslmode=disable"

## A custom name for the database that will be used as the "server" tag in the
## measurement output. If not specified, a default one generated from
## the connection address is used.
# outputaddress = "db01"

## connection configuration.
## maxlifetime - specify the maximum lifetime of a connection.
## default is forever (0s)
# max_lifetime = "0s"

## A  list of databases to explicitly ignore.  If not specified, metrics for all
## databases are gathered.  Do NOT use with the 'databases' option.
# ignored_databases = ["postgres", "template0", "template1"]

## A list of databases to pull metrics about. If not specified, metrics for all
## databases are gathered.  Do NOT use with the 'ignored_databases' option.
# databases = ["app_production", "testing"]

## Whether to use prepared statements when connecting to the database.
## This should be set to false when connecting through a PgBouncer instance
## with pool_mode set to transaction.
#prepared_statements = true
# [[instances.metrics]]
# mesurement = "sessions"
# label_fields = [ "status", "type" ]
# metric_fields = [ "value" ]
# timeout = "3s"
# request = '''
# SELECT status, type, COUNT(*) as value FROM v$session GROUP BY status, type
# '''
```

### 告警规则

![alert](./alerts.png)

[alerts](../alerts/postgresql_by_categraf.json)

### 仪表盘：

![dashboard](./postgresql.png)

[dashboard](../dashboards/postgresql_by_categraf.json)