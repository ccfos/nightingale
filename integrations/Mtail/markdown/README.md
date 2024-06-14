# mtail插件

## 简介
功能：提取日志内容，转换为监控metrics

+ 输入： 日志
+ 输出： metrics 按照mtail语法输出, 仅支持counter、gauge、histogram
+ 处理： 本质是golang的正则提取+表达式计算

## 启动
编辑mtail.toml文件, 一般每个instance需要指定不同的progs参数（不同的progs文件或者目录）,否则指标会相互干扰。
**注意**: 如果不同instance使用相同progs, 可以通过给每个instance增加labels做区分，
```
labels = { k1=v1 }
```
或
```
[instances.labels]
k1=v1
```

1. conf/inputs.mtail/mtail.toml中指定instance
```toml

[[instances]]
## 指定mtail prog的目录
progs = "/path/to/prog1"
## 指定mtail要读取的日志
logs = ["/path/to/a.log", "path/to/b.log"] 
## 指定时区
# override_timezone = "Asia/Shanghai" 
## metrics是否带时间戳，注意，这里是"true"
# emit_metric_timestamp = "true" 

```
2. 在/path/to/prog1 目录下编写规则文件
```
gauge xxx_errors
/ERROR.*/ {
    xxx_errros++
}
```

3. 一个tab中执行 `categraf --test --inputs mtail`，用于测试 
4. 另一个tab中，"/path/to/a.log" 或者 "path/to/b.log" 追加一行 ERROR，看看categraf的输出
5. 测试通过后，启动categraf

### 输入
logs参数指定要处理的日志源, 支持模糊匹配, 支持多个log文件。

### 处理规则
`progs`指定具体的规则文件目录(或文件)


## 处理规则与语法

### 处理流程
```python 
for line in lines:
  for regex in regexes:
    if match:
      do something
```

### 语法

``` golang
exported variable 

pattern { 
  action statements
} 

def decorator { 
  pattern and action statements
}
```

#### 定义指标名称
前面也提过，指标仅支持 counter gauge histogram 三种类型。
一个🌰
```mtail
counter lines
/INFO.*/ {
    lines++
}
```

注意，定义的名称只支持 C类型的命名方式(字母/数字/下划线)，**如果想使用"-" 要使用"as"导出别名**。例如，
```mtail
counter lines_total as "line-count"
```
这样获取到的就是line-count这个指标名称了

#### 匹配与计算（pattern/action)

```mtail
PATTERN {
ACTION
}
```

例子
```mtail
/foo/ {
  ACTION1
}

variable > 0 {
  ACTION2
}

/foo/ && variable > 0 {
  ACTION3
}
```
支持RE2正则匹配
```mtail
const PREFIX /^\w+\W+\d+ /

PREFIX {
  ACTION1
}

PREFIX + /foo/ {
  ACTION2
}
```

这样，ACTION1 是匹配以小写字符+大写字符+数字+空格的行，ACTION2 是匹配小写字符+大写字符+数字+空格+foo开头的行。

#### 关系运算符
+ `<` 小于 `<=` 小于等于
+ `>` 大于 `>=` 大于等于
+ `==` 相等 `!=` 不等
+ `=~` 匹配(模糊) `!~` 不匹配(模糊)
+ `||` 逻辑或 `&&` 逻辑与 `!` 逻辑非
 
#### 数学运算符
+ `|` 按位或
+ `&` 按位与
+ `^` 按位异或
+ `+ - * /` 四则运算
+ `<<` 按位左移
+ `>>` 按位右移
+ `**` 指数运算 
+ `=` 赋值
+ `++` 自增运算
+ `--` 自减运算
+ `+=` 加且赋值

#### 支持else与otherwise
```mtail
/foo/ {
ACTION1
} else {
ACTION2
}
```
支持嵌套
```mtail
/foo/ {
  /foo1/ {
     ACTION1
  }
  /foo2/ {
     ACTION2
  }
  otherwise {
     ACTION3
  }
}
```

支持命名与非命名提取

```mtail
/(?P<operation>\S+) (\S+) \[\S+\] (\S+) \(\S*\) \S+ (?P<bytes>\d+)/ {
  bytes_total[$operation][$3] += $bytes
}
```
增加常量label 
```mtail
# test.mtail
# 定义常量label env
hidden text env
# 给label 赋值 这样定义是global范围;
# 局部添加，则在对应的condition中添加
env="production"
counter line_total by logfile,env
/^(?P<date>\w+\s+\d+\s+\d+:\d+:\d+)/ {
    line_total[getfilename()][env]++
}
```
获取到的metrics中会添加上`env=production`的label 如下：
```mtail
# metrics
line_total{env="production",logfile="/path/to/xxxx.log",prog="test.mtail"} 4 1661165941788
```

如果要给metrics增加变量label，必须要使用命名提取。例如
```python
# 日志内容
192.168.0.1 GET /foo
192.168.0.2 GET /bar
192.168.0.1 POST /bar
```

``` mtail
# test.mtail
counter my_http_requests_total by log_file, verb 
/^/ +
/(?P<host>[0-9A-Za-z\.:-]+) / +
/(?P<verb>[A-Z]+) / +
/(?P<URI>\S+).*/ +
/$/ {
    my_http_requests_total[getfilename()][$verb]++
}
```

```python
# metrics
my_http_requests_total{logfile="xxx.log",verb="GET",prog="test.mtail"} 4242
my_http_requests_total{logfile="xxx.log",verb="POST",prog="test.mtail"} 42
```

命名提取的变量可以在条件中使用
```mtail
/(?P<x>\d+)/ && $x > 1 {
nonzero_positives++
}
```

#### 时间处理
不显示处理，则默认使用系统时间

默认emit_metric_timestamp="false" （注意是字符串）
```
http_latency_bucket{prog="histo.mtail",le="1"} 0
http_latency_bucket{prog="histo.mtail",le="2"} 0
http_latency_bucket{prog="histo.mtail",le="4"} 0
http_latency_bucket{prog="histo.mtail",le="8"} 0
http_latency_bucket{prog="histo.mtail",le="+Inf"} 0
http_latency_sum{prog="histo.mtail"} 0
http_latency_count{prog="histo.mtail"} 0
```

参数 emit_metric_timestamp="true" (注意是字符串)
```
http_latency_bucket{prog="histo.mtail",le="1"} 1 1661152917471
http_latency_bucket{prog="histo.mtail",le="2"} 2 1661152917471
http_latency_bucket{prog="histo.mtail",le="4"} 2 1661152917471
http_latency_bucket{prog="histo.mtail",le="8"} 2 1661152917471
http_latency_bucket{prog="histo.mtail",le="+Inf"} 2 1661152917471
http_latency_sum{prog="histo.mtail"} 3 1661152917471
http_latency_count{prog="histo.mtail"} 4 1661152917471
```

使用日志的时间
```
Aug 22 15:28:32 GET /api/v1/pods latency=2s code=200
Aug 22 15:28:32 GET /api/v1/pods latency=1s code=200
Aug 22 15:28:32 GET /api/v1/pods latency=0s code=200
```

```
histogram http_latency buckets 1, 2, 4, 8
/^(?P<date>\w+\s+\d+\s+\d+:\d+:\d+)/ {
        strptime($date, "Jan 02 15:04:05")
	/latency=(?P<latency>\d+)/ {
		http_latency=$latency
	}
}
```

日志提取的时间，一定要注意时区问题，有一个参数 `override_timezone` 可以控制时区选择，否则默认使用UTC转换。
比如我启动时指定 `override_timezone=Asia/Shanghai`, 这个时候日志提取的时间会当做东八区时间 转换为timestamp， 然后再从timestamp转换为各时区时间时 就没有问题了,如图。
![timestamp](https://cdn.jsdelivr.net/gh/flashcatcloud/categraf@main/inputs/mtail/timestamp.png)
如果不带 `override_timezone=Asia/Shanghai`, 则默认将`Aug 22 15:34:32` 当做UTC时间，转换为timestamp。 这样再转换为本地时间时，会多了8个小时, 如图。
![timestamp](https://cdn.jsdelivr.net/gh/flashcatcloud/categraf@main/inputs/mtail/timezone.png)
