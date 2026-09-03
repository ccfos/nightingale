# mtail plugin

## Introduction
Function: extract content from logs and convert it into monitoring metrics.

+ Input: logs
+ Output: metrics emitted according to mtail syntax; only counter, gauge, and histogram are supported
+ Processing: essentially Golang regex extraction plus expression evaluation

## Getting started
Edit the mtail.toml file. In general, each instance needs its own progs parameter (a different progs file or directory), otherwise metrics will interfere with each other.
**Note**: if different instances use the same progs, you can distinguish them by adding labels to each instance,
```
labels = { k1=v1 }
```
or
```
[instances.labels]
k1=v1
```

1. Specify the instance in conf/inputs.mtail/mtail.toml
```toml

[[instances]]
## Directory of the mtail progs
progs = "/path/to/prog1"
## Logs for mtail to read
logs = ["/path/to/a.log", "path/to/b.log"] 
## Time zone
# override_timezone = "Asia/Shanghai" 
## Whether metrics carry a timestamp; note that this is the string "true"
# emit_metric_timestamp = "true" 

```
2. Write rule files in the /path/to/prog1 directory
```
gauge xxx_errors
/ERROR.*/ {
    xxx_errros++
}
```

3. In one tab, run `categraf --test --inputs mtail` for testing
4. In another tab, append a line containing ERROR to "/path/to/a.log" or "path/to/b.log", and check the output of categraf
5. Once the test passes, start categraf

### Input
The logs parameter specifies the log sources to process. Glob matching and multiple log files are supported.

### Processing rules
`progs` specifies the directory (or file) of the rule files.


## Processing rules and syntax

### Processing flow
```python 
for line in lines:
  for regex in regexes:
    if match:
      do something
```

### Syntax

``` golang
exported variable 

pattern { 
  action statements
} 

def decorator { 
  pattern and action statements
}
```

#### Defining metric names
As mentioned earlier, only three metric types are supported: counter, gauge, and histogram.
An example:
```mtail
counter lines
/INFO.*/ {
    lines++
}
```

Note that names only support C-style naming (letters/digits/underscores). **If you want to use "-", export an alias with "as"**. For example,
```mtail
counter lines_total as "line-count"
```
This way the metric name you get is line-count.

#### Matching and computation (pattern/action)

```mtail
PATTERN {
ACTION
}
```

Example:
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
RE2 regular expressions are supported:
```mtail
const PREFIX /^\w+\W+\d+ /

PREFIX {
  ACTION1
}

PREFIX + /foo/ {
  ACTION2
}
```

Here, ACTION1 matches lines starting with word characters + non-word characters + digits + a space, and ACTION2 matches lines starting with word characters + non-word characters + digits + a space + foo.

#### Relational operators
+ `<` less than, `<=` less than or equal to
+ `>` greater than, `>=` greater than or equal to
+ `==` equal, `!=` not equal
+ `=~` matches (regex), `!~` does not match (regex)
+ `||` logical OR, `&&` logical AND, `!` logical NOT
 
#### Arithmetic operators
+ `|` bitwise OR
+ `&` bitwise AND
+ `^` bitwise XOR
+ `+ - * /` basic arithmetic
+ `<<` bitwise left shift
+ `>>` bitwise right shift
+ `**` exponentiation
+ `=` assignment
+ `++` increment
+ `--` decrement
+ `+=` add and assign

#### Support for else and otherwise
```mtail
/foo/ {
ACTION1
} else {
ACTION2
}
```
Nesting is supported:
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

Both named and unnamed captures are supported:

```mtail
/(?P<operation>\S+) (\S+) \[\S+\] (\S+) \(\S*\) \S+ (?P<bytes>\d+)/ {
  bytes_total[$operation][$3] += $bytes
}
```
Adding a constant label:
```mtail
# test.mtail
# Define a constant label env
hidden text env
# Assign a value to the label; defined this way it has global scope;
# to add it locally, add it inside the corresponding condition
env="production"
counter line_total by logfile,env
/^(?P<date>\w+\s+\d+\s+\d+:\d+:\d+)/ {
    line_total[getfilename()][env]++
}
```
The resulting metrics will carry the `env=production` label, as shown below:
```mtail
# metrics
line_total{env="production",logfile="/path/to/xxxx.log",prog="test.mtail"} 4 1661165941788
```

If you want to add variable labels to metrics, you must use named captures. For example:
```python
# Log content
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

Variables from named captures can be used in conditions:
```mtail
/(?P<x>\d+)/ && $x > 1 {
nonzero_positives++
}
```

#### Time handling
If not handled explicitly, the system time is used by default.

By default emit_metric_timestamp="false" (note that it is a string):
```
http_latency_bucket{prog="histo.mtail",le="1"} 0
http_latency_bucket{prog="histo.mtail",le="2"} 0
http_latency_bucket{prog="histo.mtail",le="4"} 0
http_latency_bucket{prog="histo.mtail",le="8"} 0
http_latency_bucket{prog="histo.mtail",le="+Inf"} 0
http_latency_sum{prog="histo.mtail"} 0
http_latency_count{prog="histo.mtail"} 0
```

With emit_metric_timestamp="true" (note that it is a string):
```
http_latency_bucket{prog="histo.mtail",le="1"} 1 1661152917471
http_latency_bucket{prog="histo.mtail",le="2"} 2 1661152917471
http_latency_bucket{prog="histo.mtail",le="4"} 2 1661152917471
http_latency_bucket{prog="histo.mtail",le="8"} 2 1661152917471
http_latency_bucket{prog="histo.mtail",le="+Inf"} 2 1661152917471
http_latency_sum{prog="histo.mtail"} 3 1661152917471
http_latency_count{prog="histo.mtail"} 4 1661152917471
```

Using the time from the log:
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

When extracting time from logs, always pay attention to time zone issues. The `override_timezone` parameter controls the time zone selection; otherwise UTC is used for the conversion by default.
For example, if I start with `override_timezone=Asia/Shanghai`, the time extracted from the log is treated as UTC+8 when converted to a timestamp, so converting the timestamp back to any local time zone works correctly, as shown below.
![timestamp](https://cdn.jsdelivr.net/gh/flashcatcloud/categraf@main/inputs/mtail/timestamp.png)
Without `override_timezone=Asia/Shanghai`, `Aug 22 15:34:32` is treated as UTC by default when converted to a timestamp. Converting it back to local time will then be off by 8 hours, as shown below.
![timestamp](https://cdn.jsdelivr.net/gh/flashcatcloud/categraf@main/inputs/mtail/timezone.png)
