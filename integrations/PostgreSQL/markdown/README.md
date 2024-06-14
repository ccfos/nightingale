# PostgreSQL

categraf 作为一个 client 连上 pg，采集相关指标，首先要确保用户授权。举例：

```
create user categraf with password 'categraf';
alter user categraf set default_transaction_read_only=on;
grant usage on schema public to categraf;
grant select on all tables in schema public to categraf ;
```

## 配置文件示例

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
# prepared_statements = true
#
# [[instances.metrics]]
# mesurement = "sessions"
# label_fields = [ "status", "type" ]
# metric_fields = [ "value" ]
# timeout = "3s"
# request = '''
# SELECT status, type, COUNT(*) as value FROM v$session GROUP BY status, type
# '''
```

## 仪表盘

夜莺内置了 Postgres 的仪表盘，克隆到自己的业务组下即可使用。

![20230802073729](https://download.flashcat.cloud/ulric/20230802073729.png)

## 告警规则

夜莺内置了 Postgres 的告警规则，克隆到自己的业务组下即可使用。

![20230802073753](https://download.flashcat.cloud/ulric/20230802073753.png)
