## Dameng

采用 Categraf DMDB input 连接达梦数据库并执行监控 SQL。本次使用 DM8、真实写入 20,000 行后完成采集验证。

DMDB input 需要包含达梦驱动的 Categraf 构建版本；标准开源镜像未必包含该 input。启动前先确认 `./categraf -h` 或启动日志中能够识别 `dmdb`。

## 采集配置

实例连接配置放在 `conf/input.dmdb/dmdb.toml`，查询定义放在同目录的 `metric.toml`。如需增加指标，可以新增 `[[metrics]]`。

```toml
interval = 15

[[instances]]
address = "172.0.0.1:5236"
username = "monitor"
password = "<password>"
max_open_connections = 5
max_idle_connections = 1
interval_times = 1
labels = { instance = "dmdb-01:5236" }
```

监控账号至少需要登录权限，以及读取模板所用动态性能视图和数据字典视图的权限。不要使用业务账号或把真实密码提交到仓库。

`metric.toml` 示例：

```toml

[[metrics]]
mesurement = "sessions"
label_fields = [ "state" ]
metric_fields = [ "cnt" ]
timeout = "3s"
request = '''
SELECT state, COUNT(*) as cnt FROM v$sessions GROUP BY state
'''

[[metrics]]
mesurement = "active"
metric_fields = [ "threads" ]
timeout = "3s"
request = '''
SELECT COUNT(*) as threads FROM v$threads
'''

[[metrics]]
mesurement = "latches"
metric_fields = [ "threads" ]
timeout = "3s"
request = '''
SELECT COUNT(*) as threads FROM v$latches
'''

[[metrics]]
mesurement = "lock"
metric_fields = [ "cnt" ]
timeout = "3s"
request = '''
SELECT COUNT(*) as cnt FROM v$lock
'''

[[metrics]]
mesurement = "mem"
label_fields = ["unit"]
metric_fields = [ "buffer_size", "pool_size", "total_size" ]
timeout = "3s"
request = '''
select 'MB' as unit, (select sum(n_pages * page_size)/1024/1024 from v$bufferpool) as buffer_size,(select sum(total_size)/1024/1024 from v$mem_pool) as pool_size,(select sum(n_pages * page_size)/1024/1024 from v$bufferpool)+(select sum(total_size)/1024/1024 from v$mem_pool) as total_size from  dual;
'''

[[metrics]]
mesurement = "cache"
label_fields = [ "unit", "name", "cache_size" ]
metric_fields = [ "hit_rate" ]
timeout = "3s"
request = '''
select 'GB' as unit, name, sum(page_size)*sf_get_page_size as cache_size, sum(rat_hit) /count(*)*100 as hit_rate from v$bufferpool group by name;
'''

[[metrics]]
mesurement = "tablespace"
label_fields = [ "tablespace", "unit" ]
metric_fields = [ "size", "used_size", "free_size", "usage_rate" ]
timeout = "3s"
request = '''
SELECT 'MB' as unit,
       Upper(F.TABLESPACE_NAME) as "tablespace",
       D.TOT_GROOTTE_MB as "size",
       (D.TOT_GROOTTE_MB - F.TOTAL_BYTES) as "used_size",
       Round((D.TOT_GROOTTE_MB - F.TOTAL_BYTES) / D.TOT_GROOTTE_MB * 100, 2) as "usage_rate",
       F.TOTAL_BYTES as "free_size"
FROM (
  SELECT TABLESPACE_NAME, Round(Sum(BYTES) / (1024 * 1024), 2) TOTAL_BYTES
  FROM SYS.DBA_FREE_SPACE GROUP BY TABLESPACE_NAME
) F, (
  SELECT DD.TABLESPACE_NAME, Round(Sum(DD.BYTES) / (1024 * 1024), 2) TOT_GROOTTE_MB
  FROM SYS.DBA_DATA_FILES DD GROUP BY DD.TABLESPACE_NAME
) D
WHERE D.TABLESPACE_NAME = F.TABLESPACE_NAME;
'''

```

注意：DMDB input 的配置字段当前拼写为 `mesurement`，应按插件实际字段保留，不能自行改成 `measurement`。

## 验证

```bash
./categraf --test --inputs dmdb
```

至少应看到 `dmdb_sessions_cnt`、`dmdb_mem_total_size` 和 `dmdb_tablespace_used_size`。仅能连接 5236 端口不代表监控 SQL有权限执行。
