## akamai

akamai 采集插件， 采集akamai cdn数据

## Configuration
```toml
# # collect interval ( >= 60 sec)
 interval = 300  // 默认5min, 因为akamai api 限制查询最短时间范围为5分钟，如果小于300秒，那每个周期其实查询的时间范围是一样的

# Read metrics from one or many Akamai servers
[[instances]]
# apply secret and token, ref: https://techdocs.akamai.com/developer/docs/set-up-authentication-credentials
# note: create a client, you must grant the `Reporting API` 、`Property Manager (PAPI)` at least read permission， and add ip whiltlist if you have the ip allowlist switch on
# 创建 akamai api请求client token， 并授权 `Reporting API` 、`Property Manager (PAPI)` api读权限，如果开通了白名单验证，需添加categraf出口ip白名单

client_secret = ""
host = ""
access_token = ""
client_token = ""

# default is all if it's empty, otherwise， only read metrics from the cpcodes
cp_codes=[]

# read api timeout: unit (sec)
timeout=15
# metrics_denylist: if you want to ignore some metrics, you can add it here
metrics_denylist=[]

# api rate limit (unit: req/s)
# akamai default rate limit is 12/s for each client, if you want to increase it, you can contact akamai customer support
rate_limit = 12

# akamai api version(v1/v2), default is v1 (v2 is recommended)
version = "v2"
# delay_minute: delay the start time of the each query, unit (min), suggest to set it to 5
delay_minute = 5
```