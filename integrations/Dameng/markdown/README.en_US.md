## Dameng

Use the Categraf DMDB input to connect to Dameng and execute monitoring SQL.
The real-data test used DM8 and a 20,000-row workload.

The input requires a Categraf build that includes the Dameng driver; a standard
open-source image may not contain it.

## Collector configuration
Put the instance connection in `conf/input.dmdb/dmdb.toml` and queries in
`metric.toml`. Use a dedicated monitoring account with permission to read the
dynamic performance and dictionary views referenced by the queries.

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
SELECT 'MB' as unit, Upper(F.TABLESPACE_NAME) as "tablespace",
       D.TOT_GROOTTE_MB as "size",
       (D.TOT_GROOTTE_MB - F.TOTAL_BYTES) as "used_size",
       Round((D.TOT_GROOTTE_MB - F.TOTAL_BYTES) / D.TOT_GROOTTE_MB * 100, 2) as "usage_rate",
       F.TOTAL_BYTES as "free_size"
FROM (SELECT TABLESPACE_NAME, Round(Sum(BYTES)/(1024*1024), 2) TOTAL_BYTES
      FROM SYS.DBA_FREE_SPACE GROUP BY TABLESPACE_NAME) F,
     (SELECT TABLESPACE_NAME, Round(Sum(BYTES)/(1024*1024), 2) TOT_GROOTTE_MB
      FROM SYS.DBA_DATA_FILES GROUP BY TABLESPACE_NAME) D
WHERE D.TABLESPACE_NAME = F.TABLESPACE_NAME;
'''

```

The DMDB input currently spells the field `mesurement`; keep that spelling.
Validate with `./categraf --test --inputs dmdb` and check
`dmdb_sessions_cnt`, `dmdb_mem_total_size`, and
`dmdb_tablespace_used_size`.
