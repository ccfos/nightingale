

# GCP 指标获取插件

## 需要权限
```shell
https://www.googleapis.com/auth/monitoring.read
```

## 配置
```toml
#采集周期，建议 >= 1分钟
interval=60
[[instances]]
#配置 project_id
project_id="your-project-id"
#配置认证的key文件
credentials_file="/path/to/your/key.json"
#或者配置认证的JSON
credentials_json="xxx"

# 指标的end time = now - delay
#delay="2m"
# 指标的start time = now - deley - period
#period="1m"
# 过滤器
#filter="metric.type=\"compute.googleapis.com/instance/cpu/utilization\" AND resource.labels.zone=\"asia-northeast1-a\""
# 请求超时时间
#timeout="5s"
# 指标列表的缓存时长 ，filter为空时 启用
#cache_ttl="1h"

# 给gce的instance_name 取个别名,放到label中
#gce_host_tag="xxx"
# 每次最多有多少请求同时发起
#request_inflight=30

# request_inflight 取值(0,100]
# 想配置更大的值 ,前提是你知道你在做什么
force_request_inflight= 200
```
