## AppDynamics

The AppDynamics collection plugin collects AppDynamics data.

## Configuration

```toml
#interval=15s

[[instances]]
#url_base = "http://{{.ip}}:{{.port}}/a.json?metric-path={{.metric_path}}&time-range-type=BETWEEN_TIMES&start-time={{.start_time}}&end-time={{.end_time}}&output=JSON"
#url_vars = [
#    { ip="127.0.0.1", port="8090", application="cms", metric_path="Application Infrastructure Performance|AdminServer|Individual Nodes|xxxxx|Agent|App|Availability", start_time="$START_TIME", end_time="$END_TIME"},
#]

# # Specify which keys in url_vars are attached as final labels
# url_var_label_keys= []

# # Extract variables from the URL
# url_label_key="instance"
# url_label_value="{{.Host}}"
# # Custom http headers
#headers = { Authorization="", X-Forwarded-For="", Host=""}
# # Timeout for each request
#timeout="5s"

# # precision of start-time and end-time
#precision="ms"

## basic auth
#username=""
#password=""

# #  endtime = now - delay
#delay = "1m"
# # starttime = now - delay - period = endtime - period
#period = "1m"

# # Extra labels to add
#labels = {application="cms"}
# # Which metrics to filter from the response
filters = ["current", "max", "min", "value","sum", "count"]

# # Limit concurrent requests, i.e. the maximum number of in-flight requests
# # Default range (0,100)
#request_inflight= 10
## Force enabling more than 100 concurrent requests (not recommended)
# force_request_inflight =  1000

# # Whether to enable tls
# use_tls = true
# # Minimum tls version
## tls_min_version = "1.2"
# # Path to the tls ca certificate
## tls_ca = "/etc/categraf/ca.pem"
# # Path to the tls cert
## tls_cert = "/etc/categraf/cert.pem"
# # Path to the tls key
## tls_key = "/etc/categraf/key.pem"
# # Whether to skip certificate verification
## insecure_skip_verify = true

```
