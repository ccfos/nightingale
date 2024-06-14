# aliyun plugin

## 简介

使用[categraf](https://github.com/flashcatcloud/categraf)中[aliyun](https://github.com/flashcatcloud/categraf/tree/main/inputs/aliyun)插件拉取阿里云云监控的数据（通过 OpenAPI）。

## 授权

获取凭证 [https://usercenter.console.aliyun.com/#/manage/ak](https://usercenter.console.aliyun.com/#/manage/ak)
RAM 用户授权。RAM 用户调用云监控 API 前，需要所属的阿里云账号将权限策略授予对应的 RAM 用户，参见 [RAM 用户权限](https://help.aliyun.com/document_detail/43170.html?spm=a2c4g.11186623.0.0.30c841feqsoAAn)。
可以在 [授权页面](https://ram.console.aliyun.com/permissions) 新增授权，选择对应的用户，授予云监控只读权限 `AliyunCloudMonitorReadOnlyAccess`, 并为授予权限的用户创建accessKey 即可。

## Categraf中conf/input.aliyun/cloud.toml配置文件：

```toml
# # categraf采集周期，阿里云指标的粒度一般是60秒，建议设置不要少于60秒
interval = 120
[[instances]]
## 阿里云资源所处的region
## endpoint region 参考 https://help.aliyun.com/document_detail/28616.html#section-72p-xhs-6qt
region="cn-beijing"
endpoint="metrics.cn-hangzhou.aliyuncs.com"
## 填入你的access_key_id
access_key_id=""
## 填入你的access_key_secret
access_key_secret=""

## 可能无法获取当前最新指标，这个指标是指监控指标的截止时间距离现在多久
delay="50m"
## 阿里云指标的最小粒度，60s 是推荐值，再小了部分指标不支持
period="60s"
## 指标所属的namespace ,为空，则表示所有空间指标都要采集
## namespace 参考 https://help.aliyun.com/document_detail/163515.htm?spm=a2c4g.11186623.0.0.44d65c58mhgNw3
namespaces=["acs_ecs_dashboard"]
## 过滤某个namespace下的一个或多个指标
## metric name 参考 https://help.aliyun.com/document_detail/163515.htm?spm=a2c4g.11186623.0.0.401d15c73Z0dZh
## 参考页面中的Metric Id 填入下面的metricName ,页面中包含中文的Metric Name对应接口中的Description
[[instances.metric_filters]]
namespace=""
metric_names=["cpu_cores","vm.TcpCount", "cpu_idle"]

# 阿里云查询指标接口的QPS是50， 这里默认设置为一半
ratelimit=25
# 查询指定namesapce指标后, namespace/metric_name等meta信息会缓存起来，catch_ttl 是指标的缓存时间
catch_ttl="1h"
# 每次请求阿里云endpoint的超时时间
timeout="5s"
```

## 效果图

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
