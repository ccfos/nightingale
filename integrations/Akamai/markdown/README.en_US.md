## akamai

Akamai collection plugin, used to collect Akamai CDN data.

## Configuration
```toml
# # collect interval ( >= 60 sec)
 interval = 300  // Default is 5min. The Akamai API limits the minimum query time range to 5 minutes, so with a value below 300 seconds each cycle would actually query the same time range

# Read metrics from one or many Akamai servers
[[instances]]
# apply secret and token, ref: https://techdocs.akamai.com/developer/docs/set-up-authentication-credentials
# note: create a client, you must grant the `Reporting API` 、`Property Manager (PAPI)` at least read permission， and add ip whiltlist if you have the ip allowlist switch on
# Create an Akamai API client token and grant it read permission on the `Reporting API` and `Property Manager (PAPI)` APIs. If IP allowlist verification is enabled, add categraf's egress IP to the allowlist

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
