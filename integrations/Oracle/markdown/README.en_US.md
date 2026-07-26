# Oracle plugin

The Oracle plugin monitors Oracle databases. By default it cannot run on Windows. If your Oracle database is deployed on Windows, that's not a problem: you can use a Categraf instance deployed on Linux to remotely monitor the Oracle database running on Windows.

The real-data validation used Oracle Database Free 26ai and a 20,000-row
workload.

## Monitoring account

Use a dedicated account. This broad baseline is convenient for validation;
production deployments can grant only the views queried by `metric.toml`:

```sql
CREATE USER categraf IDENTIFIED BY "<password>";
GRANT CREATE SESSION TO categraf;
GRANT SELECT_CATALOG_ROLE TO categraf;
```

## Instance configuration

Put the instance connection in `conf/input.oracle/oracle.toml` and the query
definitions in `conf/input.oracle/metric.toml`:

```toml
interval = 15

[[instances]]
address = "oracle.example.com:1521/FREE"
username = "categraf"
password = "<password>"
is_sys_dba = false
is_sys_oper = false
disable_connection_pool = false
max_open_connections = 5
conn_max_idle_time = "15m"
conn_max_lifetime = "24h"
labels = { env = "prod" }
```

The final address segment is the service name, not necessarily the SID. The
dashboard selects `oracle_up` by its `address` label.

The plugin executes [these SQL statements](https://github.com/flashcatcloud/categraf/blob/main/conf/input.oracle/metric.toml), parses their results, and reports metrics.

Take one of them as an example:

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

- mesurement: the metric category; keep this spelling because it is the field
  currently parsed by the Oracle input
- label_fields: fields used as labels
- metric_fields: fields used as metrics; since they are used as metric values, these fields must be numeric
- field_to_append: this field is appended to the metric_name, becoming part of the metric_name
- timeout: query timeout
- request: the SQL statement to execute

If a metric you want to monitor is not collected by default, simply add a custom `[[metrics]]` configuration section.

Validate with `./categraf --test --inputs oracle`. Confirm `oracle_up == 1`,
sessions, activity, tablespace, and sysmetric data. Data Guard, ASM, lock, and
slow-SQL metrics appear only when the corresponding feature or event exists.
