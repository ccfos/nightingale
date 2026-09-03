# SQL Server

Categraf's SQL Server input reads DMVs and performance counters. The real-data
test used SQL Server 2022.

## Monitoring account

```sql
USE [master];
GO
CREATE LOGIN [categraf] WITH PASSWORD = N'<strong-password>';
GO
GRANT VIEW SERVER STATE TO [categraf];
GRANT VIEW ANY DEFINITION TO [categraf];
GO
```

SQL Server 2022 and later also require performance-state access for
[performance DMVs](https://learn.microsoft.com/sql/relational-databases/system-dynamic-management-objects/system-dynamic-management-objects):

```sql
GRANT VIEW SERVER PERFORMANCE STATE TO [categraf];
GO
```

## Categraf configuration

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

Remove the two exclusions when Always On is configured and should be
monitored. Validate with `./categraf --test --inputs sqlserver`, then confirm
`sqlserver_up == 1` and a non-empty `sql_instance` label. A successful TCP
connection alone is not a collection test.
