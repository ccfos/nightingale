## SQL变量

| 字段名 | 含义  | 使用场景 |
| ----  | ---- |  ----  | 
|database|数据库|无|
|table|表名||
|time_field|时间戳的字段||
|query|查询条件|日志原文|
|from|开始时间||
|to|结束时间||
|aggregation|聚合算法|时序图|
|field|聚合的字段|时序图|
|limit|分页参数|日志原文|
|offset|分页参数|日志原文|
|interval|直方图的时间粒度|直方图|

## 日志原文
### 直方图

```
# 如何计算interval的值
max := 60 // 最多60个柱子
interval := ($to-$from) / max
interval = interval - interval%10
if interval <= 0 {
	interval = 60
}
```

```
SELECT count() as cnt,
	FLOOR(UNIX_TIMESTAMP($time_field) / $interval) * $interval AS __ts__ 
		FROM $table
	WHERE $time_field BETWEEN FROM_UNIXTIME($from) AND FROM_UNIXTIME($to)
	GROUP BY __ts__;
```

```
{
	"database":"$database",
	"sql":"$sql",
	"keys:": {
		"valueKey":"cnt",
		"timeKey":"__ts__"
	}
}
```

### 日志原文

```
SELECT * from $table
	WHERE $time_field BETWEEN FROM_UNIXTIME($from) AND FROM_UNIXTIME($to)
	ORDER by $time_filed
	LIMIT $limit OFFSET $offset;
```

```
{
	"database":"$database",
	"sql":"$sql"
}
```

## 时序图

### 日志行数

```
SELECT COUNT() AS cnt, DATE_FORMAT(date, '%Y-%m-%d %H:%i:00') AS __ts__ 
	FROM nginx_access_log
	WHERE $time_field BETWEEN FROM_UNIXTIME($from) AND FROM_UNIXTIME($to)
	GROUP BY __ts__
```

```
{
	"database":"$database",
	"sql":"$sql",
	"keys:": {
		"valueKey":"cnt",
		"timeKey":"__ts__"
	}
}
```

### max/min/avg/sum

```
SELECT $aggregation($field) AS series, DATE_FORMAT(date, '%Y-%m-%d %H:%i:00') AS __ts__ 
	FROM nginx_access_log
	WHERE $time_field BETWEEN FROM_UNIXTIME($from) AND FROM_UNIXTIME($to)
	GROUP BY __ts__
```

```
{
	"database":"$database",
	"sql":"$sql",
	"keys:": {
		"valueKey":"series",
		"timeKey":"__ts__"
	}
}
```


### 分位值

```
SELECT percentile($field, 0.95) AS series, DATE_FORMAT(date, '%Y-%m-%d %H:%i:00') AS __ts__ 
	FROM nginx_access_log
	WHERE $time_field BETWEEN FROM_UNIXTIME($from) AND FROM_UNIXTIME($to)
	GROUP BY __ts__
```

```
{
	"database":"$database",
	"sql":"$sql",
	"keys:": {
		"valueKey":"series",
		"timeKey":"__ts__"
	}
}
```