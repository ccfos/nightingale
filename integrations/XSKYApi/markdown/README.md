# XSKY Api

XSKY api

## Configations

```toml
# # collect interval
# interval = 15
#
[[instances]]
# # append some labels for series
# labels = { region="cloud", product="n9e" }

# # interval = global.interval * interval_times
# interval_times = 1

## must be one of oss/gfs/eus
dss_type = "oss"

## URL of each server in the service's cluster
servers = [
    #"http://x.x.x.x:xx"
]

## Set response_timeout (default 5 seconds)
response_timeout = "5s"

xms_auth_tokens = [
    #"xxxxxxxxxxxxxxx"
]


```