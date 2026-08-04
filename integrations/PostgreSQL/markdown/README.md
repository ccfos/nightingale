# PostgreSQL

categraf 作为一个 client 连上 PostgreSQL 采集指标，建议创建专用监控账号：

```sql
CREATE USER categraf WITH PASSWORD '<password>';
GRANT pg_monitor TO categraf;
ALTER USER categraf SET default_transaction_read_only = on;
```

`pg_monitor` 用于读取 PostgreSQL 监控视图。只有配置自定义业务表查询时，才需要再授予对应 schema/table 的 `USAGE` 和 `SELECT`。

## 配置文件示例

```toml
[[instances]]
address = "host=192.168.11.181 port=5432 user=categraf password=<password> sslmode=disable"
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
# prepared_statements = true
#
# [[instances.metrics]]
# measurement = "sessions"
# label_fields = [ "state" ]
# metric_fields = [ "value" ]
# timeout = "3s"
# request = '''
# SELECT COALESCE(state, 'unknown') AS state,
#        COUNT(*)::double precision AS value
# FROM pg_stat_activity
# GROUP BY COALESCE(state, 'unknown')
# '''
```
