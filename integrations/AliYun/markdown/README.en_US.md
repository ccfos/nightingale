# aliyun plugin

## Introduction

Use the [aliyun](https://github.com/flashcatcloud/categraf/tree/main/inputs/aliyun) plugin in [categraf](https://github.com/flashcatcloud/categraf) to pull Alibaba Cloud CloudMonitor data (via OpenAPI).

## Authorization

Obtain credentials at [https://usercenter.console.aliyun.com/#/manage/ak](https://usercenter.console.aliyun.com/#/manage/ak).
RAM user authorization: before a RAM user can call the CloudMonitor API, the Alibaba Cloud account it belongs to must grant the corresponding permission policy to that RAM user; see [RAM user permissions](https://help.aliyun.com/document_detail/43170.html?spm=a2c4g.11186623.0.0.30c841feqsoAAn).
You can add an authorization on the [authorization page](https://ram.console.aliyun.com/permissions): select the corresponding user, grant the CloudMonitor read-only permission `AliyunCloudMonitorReadOnlyAccess`, and create an accessKey for the authorized user.

## Categraf configuration file conf/input.aliyun/cloud.toml:

```toml
# # categraf collection interval; Alibaba Cloud metrics generally have a granularity of 60 seconds, so it is recommended not to set this below 60 seconds
interval = 120
[[instances]]
## The region where your Alibaba Cloud resources are located
## For endpoint region, see https://help.aliyun.com/document_detail/28616.html#section-72p-xhs-6qt
region="cn-beijing"
endpoint="metrics.cn-hangzhou.aliyuncs.com"
## Fill in your access_key_id
access_key_id=""
## Fill in your access_key_secret
access_key_secret=""

## The very latest metrics may not be available; this value is how far back from now the metric cutoff time is
delay="50m"
## The minimum granularity of Alibaba Cloud metrics; 60s is the recommended value, some metrics do not support smaller values
period="60s"
## The namespace the metrics belong to; if empty, metrics from all namespaces will be collected
## For namespace, see https://help.aliyun.com/document_detail/163515.htm?spm=a2c4g.11186623.0.0.44d65c58mhgNw3
namespaces=["acs_ecs_dashboard"]
## Filter one or more metrics under a namespace
## For metric name, see https://help.aliyun.com/document_detail/163515.htm?spm=a2c4g.11186623.0.0.401d15c73Z0dZh
## Fill in the Metric Id from the reference page as metricName below; the Metric Name containing Chinese on that page corresponds to the Description field in the API
[[instances.metric_filters]]
namespace=""
metric_names=["cpu_cores","vm.TcpCount", "cpu_idle"]

# The QPS of the Alibaba Cloud metric query API is 50; the default here is set to half of that
ratelimit=25
# After querying metrics of a specified namespace, meta information such as namespace/metric_name is cached; catch_ttl is the cache TTL for metrics
catch_ttl="1h"
# Timeout for each request to the Alibaba Cloud endpoint
timeout="5s"
```

## Screenshots

### aliyun ecs

![ecs](http://download.flashcat.cloud/uPic/R6LOcO.jpg)

### aliyun rds

![rds](http://download.flashcat.cloud/uPic/rds.png)

### aliyun redis

![redis](http://download.flashcat.cloud/uPic/redis.png)

### aliyun slb

![slb](http://download.flashcat.cloud/uPic/slb.png)

### aliyun waf

![waf](http://download.flashcat.cloud/uPic/waf.png)
