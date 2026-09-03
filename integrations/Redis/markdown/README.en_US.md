# redis

The redis monitoring principle is simple: connect to redis, execute the info command, parse the result, and organize it into monitoring data to report.

## Configuration

The redis plugin configuration lives in `conf/input.redis/redis.toml`. The simplest configuration looks like this:

```toml
[[instances]]
address = "127.0.0.1:6379"
username = ""
password = ""
labels = { instance="n9e-10.23.25.2:6379" }
```

To monitor multiple redis instances, just add more instances:

```toml
[[instances]]
address = "10.23.25.2:6379"
username = ""
password = ""
labels = { instance="n9e-10.23.25.2:6379" }

[[instances]]
address = "10.23.25.3:6379"
username = ""
password = ""
labels = { instance="n9e-10.23.25.3:6379" }
```

It is recommended to attach an instance label via the labels configuration, which makes it easier to reuse monitoring dashboards later.

## How to monitor a redis cluster

In fact, monitoring a redis cluster still means monitoring each individual redis instance.

If a redis cluster has 3 instances, a business application making a request may randomly hit any one of the instances, which is fine. But for a monitoring client, you obviously want to fetch data from all instances.

Of course, when multiple redis instances form a cluster, we want some identifier for that cluster. This can be done via labels — for example, add a redis_clus label to each instance, with the cluster name as its value.


# redis_sentinel
forked from [telegraf/redis_sentinel](https://github.com/influxdata/telegraf/tree/master/plugins/inputs/redis_sentinel)
