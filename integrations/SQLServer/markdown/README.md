# SQL Server

Categraf SQL Server input 基于 Telegraf SQL Server input，使用 DMV 和性能计数器采集本地部署的 SQL Server。本次已使用 SQL Server 2022 和 Categraf 完成真实采集验证。

## 创建只读监控账号

在 `master` 数据库执行：

```sql
USE [master];
GO

CREATE LOGIN [categraf] WITH PASSWORD = N'<strong-password>';
GO

GRANT VIEW SERVER STATE TO [categraf];
GRANT VIEW ANY DEFINITION TO [categraf];
GO
```

SQL Server 2022 及以上还需要为[性能 DMV](https://learn.microsoft.com/sql/relational-databases/system-dynamic-management-objects/system-dynamic-management-objects) 授权：

```sql
GRANT VIEW SERVER PERFORMANCE STATE TO [categraf];
GO
```

不要在文档或仓库中保存真实密码。若企业安全策略要求更细粒度授权，可结合实际启用的查询进一步收敛权限。

## Categraf 配置

配置文件为 `conf/input.sqlserver/sqlserver.toml`：

```toml
interval = 15

[[instances]]
servers = [
  "Server=10.19.1.1;Port=1433;User Id=categraf;Password=<strong-password>;app name=categraf;log=1;"
]
auth_method = "connection_string"
database_type = "SQLServer"
include_query = []
exclude_query = [
  "SQLServerAvailabilityReplicaStates",
  "SQLServerDatabaseReplicaStates"
]
health_metric = true
```

如果实例配置了 Always On，可从 `exclude_query` 删除两项副本查询。未配置 Always On 时排除它们，可以避免无意义的权限或空结果问题。

## 验证

```bash
./categraf --test --inputs sqlserver
```

在时序库中确认 `sqlserver_up` 为 1，并且带有模板使用的 `sql_instance` 标签。只验证 TCP 1433 可连接不能证明采集成功。
