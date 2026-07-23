

# GCP Metrics Collection Plugin

## Required Permissions
```shell
https://www.googleapis.com/auth/monitoring.read
```

## Configuration
```toml
# collect interval, recommended to be >= 1 minute
interval=60
[[instances]]
# set the project_id
project_id="your-project-id"
# set the credentials key file
credentials_file="/path/to/your/key.json"
# or set the credentials JSON directly
credentials_json="xxx"

# metric end time = now - delay
#delay="2m"
# metric start time = now - deley - period
#period="1m"
# filter
#filter="metric.type=\"compute.googleapis.com/instance/cpu/utilization\" AND resource.labels.zone=\"asia-northeast1-a\""
# request timeout
#timeout="5s"
# cache TTL for the metric list; only takes effect when filter is empty
#cache_ttl="1h"

# give the GCE instance_name an alias and put it into a label
#gce_host_tag="xxx"
# maximum number of concurrent requests in flight
#request_inflight=30

# request_inflight valid range is (0,100]
# set a larger value only if you know what you are doing
force_request_inflight= 200
```
