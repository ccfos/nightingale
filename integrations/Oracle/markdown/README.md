# Oracle plugin

Oracle 插件，用于监控 Oracle 数据库。默认无法跑在 Windows 上。如果你的 Oracle 部署在 Windows 上，也没问题，使用部署在 Linux 上的 Categraf 远程监控 Windows 上的 Oracle，也行得通。

本次使用 Oracle Database Free 26ai、写入 20,000 行后，通过 Categraf Oracle input 完成验证。

## 监控账号

建议创建专用账号。下面是便于验证的基础授权示例，生产环境可以按 `metric.toml` 实际查询的视图进一步收敛：

```sql
CREATE USER categraf IDENTIFIED BY "<password>";
GRANT CREATE SESSION TO categraf;
GRANT SELECT_CATALOG_ROLE TO categraf;
```

不同 Oracle 版本、CDB/PDB、Data Guard 和 ASM 环境需要的视图不同。若某条 SQL 报 `ORA-00942` 或 `ORA-01031`，应补充该视图的最小读取权限，而不是改用业务账号。

## 实例配置

实例连接配置为 `conf/input.oracle/oracle.toml`，查询定义单独放在同目录的 `metric.toml`：

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
interval_times = 1
labels = { env = "prod" }
```

`address` 的最后一段是 service name，不一定是 SID。模板使用 `oracle_up` 的 `address` 标签筛选实例，不要在采集侧删除或统一覆盖这个标签。

## 自定义查询

Oracle input 的核心原理是执行 [metric.toml](https://github.com/flashcatcloud/categraf/blob/main/conf/input.oracle/metric.toml) 中的 SQL，再把数值列转换为指标。

以其中一个为例：

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

- measurement：指标类别
- label_fields：作为 label 的字段
- metric_fields：作为 metric 的字段，因为是作为 metric 的字段，所以这个字段的值必须是数字
- field_to_append：表示这个字段附加到 metric_name 后面，作为 metric_name 的一部分
- timeout：超时时间
- request：具体查询的 SQL 语句

如果你想监控的指标，默认没有采集，只需要增加自定义的 `[[metrics]]` 配置即可。

注意：当前 Oracle input 配置结构中的字段拼写是 `mesurement`，应按 Categraf 实际配置保留。写成 `measurement` 会导致自定义查询名称不能被正确解析。

## 验证

```bash
./categraf --test --inputs oracle
```

至少确认 `oracle_up` 为 1，并检查 sessions、activity、tablespace 和 sysmetric 指标。Data Guard、ASM、锁和慢 SQL 面板只有对应功能或事件存在时才会返回数据。
