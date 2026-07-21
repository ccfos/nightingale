# Cloudflare
Monitors Cloudflare CDN service metrics. For free Cloudflare accounts, only a small number of metrics will be reported, which is expected.

# Configuration Example
```
# # collect interval
interval = 60
# # endpoint region, see https://developers.cloudflare.com/analytics/graphql-api
endpoint="https://api.cloudflare.com/client/v4/graphql"


# There are two ways of api authentication
# the one is: cf_api_token, the other are cf_api_email + cf_api_key
#cloudflare api key, works with api_email flag
cf_api_key=""
# cloudflare api email, works with api_key flag 
# https://support.cloudflare.com/hc/en-us/articles/200167836-Managing-API-Tokens-and-Keys
cf_api_email=""

# cloudflare api token (recommended)
# https://developers.cloudflare.com/analytics/graphql-api/getting-started/authentication/api-token-auth , just pick the `Read analytics and logs` permission template
cf_api_token="apply for a token via the link above"

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
