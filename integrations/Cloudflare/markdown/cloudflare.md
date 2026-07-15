# Cloudflare
cloudflare cdn服务指标监控, 对于cloudflare免费账号，只会有少了指标上报，属于正常情况

# 配置示例
```
# # collect interval
interval = 60
# # endpoint region 参考 https://developers.cloudflare.com/analytics/graphql-api
endpoint="https://api.cloudflare.com/client/v4/graphql"


# There are two ways of api authentication
# the one is: cf_api_token, the other are cf_api_email + cf_api_key
#cloudflare api key, works with api_email flag
cf_api_key=""
# cloudflare api email, works with api_key flag 
# https://support.cloudflare.com/hc/en-us/articles/200167836-Managing-API-Tokens-and-Keys
cf_api_email=""

# cloudflare api token (推荐方式)
# https://developers.cloudflare.com/analytics/graphql-api/getting-started/authentication/api-token-auth ， 选 `Read analytics and logs` 这个权限模板即可
cf_api_token="根据上面链接，申请token"

#cloudflare zones to export, the default value is all zones under the account
# format: zoneIDs
cf_zones=[]

#cloudflare zones to exclude
# format: zoneIDs
cf_exclude_zones=[]

#metrics to not expose
metrics_denylist=[]

#scrape delay in seconds, defaults to 120 s
scrape_delay=120

# request clodflare timeout (unit: s)
timeout=10
```
