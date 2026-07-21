# Oracle plugin

The Oracle plugin monitors Oracle databases. By default it cannot run on Windows. If your Oracle database is deployed on Windows, that's not a problem: you can use a Categraf instance deployed on Linux to remotely monitor the Oracle database running on Windows.

The core monitoring principle of the Oracle plugin is to execute [these SQL statements](https://github.com/flashcatcloud/categraf/blob/main/conf/input.oracle/metric.toml), parse the results, and report them to the monitoring server.

Take one of them as an example:

```toml
[[metrics]]
measurement = "activity"
metric_fields = [ "value" ]
field_to_append = "name"
timeout = "3s"
request = '''
SELECT name, value FROM v$sysstat WHERE name IN ('parse count (total)', 'execute count', 'user commits', 'user rollbacks')
'''
```

- measurement: the metric category
- label_fields: fields used as labels
- metric_fields: fields used as metrics; since they are used as metric values, these fields must be numeric
- field_to_append: this field is appended to the metric_name, becoming part of the metric_name
- timeout: query timeout
- request: the SQL statement to execute

If a metric you want to monitor is not collected by default, simply add a custom `[[metrics]]` configuration section.
