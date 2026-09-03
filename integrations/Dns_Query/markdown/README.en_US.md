# Use Cases
Typically used to monitor the responsiveness of DNS servers, helping operations teams quickly locate network issues.

# Deployment Scenario
There is no need to enable this plugin on every virtual machine. It is recommended to enable it on a single dedicated or shared virtual machine.

# Configuration Scenario
```
This configuration enables or defines the following capabilities:
Use the local DNS server to check domain resolution quality.
Use external DNS servers to check domain resolution quality.
Perform DNS queries with different record types.
Set a 5-second timeout for each query.
Add custom labels, which can be used to filter data and deliver more precise alert notifications.
Add the domains you want to query in the domains field — usually your company's business system domains or third-party dependencies.
```

# Configuration Reference
The configuration file is conf/input.dns_query/dns_query.toml.
```toml
[[instances]]
auto_detect_local_dns_server = false # Whether to auto-detect the local DNS server. When set to true, the plugin reads the locally configured DNS servers from /etc/resolv.conf

servers = ["114.114.114.114"] # List of DNS servers to query

network = "udp" # Protocol used for queries, udp or tcp

domains = ["flashcat.cloud"] # List of domains to query

record_type="NS" # DNS record type to query

port = 53 # DNS server port

timeout = 2 # DNS query timeout in seconds

expect_query_ips = { "domain" = ["IP1", "IP2", ...] } # Expected IP address configuration for domain hijacking detection
```
1. record_type supports the following types:

- A: IPv4 address record, returns the IPv4 address of the domain
- AAAA: IPv6 address record, returns the IPv6 address of the domain
- CNAME: alias record, returns the canonical name of the domain
- MX: mail exchange record, returns mail server information
- NS: name server record, returns the authoritative DNS servers of the domain
- PTR: pointer record, used for reverse DNS lookups (IP address to domain)
- TXT: text record, returns text information associated with the domain
- SOA: start of authority record, returns authoritative information about the domain
- SPF: sender policy framework record, used for email anti-spoofing
- SRV: service record, specifies the location of services
- ANY: any record type, returns all available records

2. expect_query_ips must satisfy the following conditions
- Only effective when record_type is A or AAAA
- The IP list should be as complete as possible, covering all legitimate IP addresses of the domain
- It is recommended to update the IP list regularly to keep it accurate


# Editing the dns_query.toml Configuration

``` The following file content is provided for reference
[root@aliyun input.dns_query]# cat dns_query.toml
# # collect interval
# interval = 15

[[instances]]
# # append some labels for series
labels = { cloud="huaweicloud", region="huabei-beijing-4",azone="az1", product="n9e" }

# # interval = global.interval * interval_times
# interval_times = 1

# #
auto_detect_local_dns_server  = true

### A record

## servers to query
servers = ["223.5.5.5","114.114.114.114","119.29.29.29"]

## Network is the network protocol name.
# network = "udp"

## Domains or subdomains to query.
domains = ["www.huaweicloud.com", "www.baidu.com", "www.tapd.cn"]

## Query record type.
## Possible values: A, AAAA, CNAME, MX, NS, PTR, TXT, SOA, SPF, SRV.
record_type = "A"

## Dns server port.
# port = 53

## Query timeout in seconds.
timeout = 5


### CNAME record

[[instances]]
# # append some labels for series
labels = { cloud="huaweicloud", region="huabei-beijing-4",azone="az1", product="n9e" }

# # interval = global.interval * interval_times
# interval_times = 1

# #
auto_detect_local_dns_server  = false

## servers to query
servers = ["223.5.5.5","114.114.114.114","119.29.29.29"]

## Network is the network protocol name.
# network = "udp"

## Domains or subdomains to query.
domains = ["www.huaweicloud.com", "www.baidu.com", "www.tapd.cn"]

## Query record type.
## Possible values: A, AAAA, CNAME, MX, NS, PTR, TXT, SOA, SPF, SRV.
record_type = "CNAME"

## Dns server port.
# port = 53

## Query timeout in seconds.
timeout = 5


### NS record

[[instances]]
# # append some labels for series
labels = { cloud="huaweicloud", region="huabei-beijing-4",azone="az1", product="n9e" }

# # interval = global.interval * interval_times
# interval_times = 1

# #
auto_detect_local_dns_server  = false

## servers to query
servers = ["223.5.5.5","114.114.114.114","119.29.29.29"]

## Network is the network protocol name.
# network = "udp"

## Domains or subdomains to query.
domains = ["www.huaweicloud.com", "www.baidu.com", "www.tapd.cn"]

## Query record type.
## Possible values: A, AAAA, CNAME, MX, NS, PTR, TXT, SOA, SPF, SRV.
record_type = "NS"

## Dns server port.
# port = 53

## Query timeout in seconds.
timeout = 5
```

# Testing the Configuration
```
./categraf --test --inputs dns_query
....... A records work the same way and are omitted here
20:51:34 dns_query_rcode_value agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud domain=www.tapd.cn product=n9e record_type=CNAME region=huabei-beijing-4 server=119.29.29.29 0
20:51:34 dns_query_result_code agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud domain=www.tapd.cn product=n9e record_type=CNAME region=huabei-beijing-4 server=119.29.29.29 0
20:51:34 dns_query_query_time_ms agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud domain=www.tapd.cn product=n9e record_type=CNAME region=huabei-beijing-4 server=119.29.29.29 33.500371

20:51:34 dns_query_rcode_value agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud domain=www.baidu.com product=n9e record_type=CNAME region=huabei-beijing-4 server=119.29.29.29 0
20:51:34 dns_query_result_code agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud domain=www.baidu.com product=n9e record_type=CNAME region=huabei-beijing-4 server=119.29.29.29 0
20:51:34 dns_query_query_time_ms agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud domain=www.baidu.com product=n9e record_type=CNAME region=huabei-beijing-4 server=119.29.29.29 34.328242

20:51:34 dns_query_rcode_value agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud domain=www.huaweicloud.com product=n9e record_type=CNAME region=huabei-beijing-4 server=119.29.29.29 0
20:51:34 dns_query_result_code agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud domain=www.huaweicloud.com product=n9e record_type=CNAME region=huabei-beijing-4 server=119.29.29.29 0
20:51:34 dns_query_query_time_ms agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud domain=www.huaweicloud.com product=n9e record_type=CNAME region=huabei-beijing-4 server=119.29.29.29
.....

```
# Restarting the Service
```
Restart the categraf service to apply the changes
systemctl daemon-reload && systemctl restart categraf && systemctl status categraf

Check the startup logs for errors
journalctl -f -n 500 -u categraf | grep "E\!" | grep "W\!"
```

# Verifying the Data
After waiting 1-2 minutes, the data will show up in the charts, as shown below:
![image](https://user-images.githubusercontent.com/12181410/220353480-e17a7822-7ccc-4fdf-b18b-a0be84cd5550.png)

# Metric Reference
1. dns_query_time_ms is the query response time, in ms.
2. dns_query_result_value is a categraf-defined query result: 0 means success, 1 means timeout, 2 means an error occurred.
3. dns_query_rcode_value is the status code in the DNS response, indicating how the query was handled:
    - 0 (NOERROR): the query succeeded with no errors
    - 1 (FORMERR): format error, the DNS server could not understand the query
    - 2 (SERVFAIL): server failure, the DNS server ran into an internal error
    - 3 (NXDOMAIN): the domain does not exist
    - 4 (NOTIMP): the query type is not supported
    - 5 (REFUSED): the query was refused, usually for policy reasons. In monitoring, an RCODE of 0 means the query succeeded; any other value usually indicates a problem worth investigating
4. Once expect_query_ips is configured for a domain, categraf compares the IP list in the response against the configured IP list, producing a new metric dns_query_status_change: 0 means the responded IPs match the expected IPs, 1 means they differ. When they differ, another metric is generated: dns_query_status_change_detail{ips="responded IP list", diff="responded IP list minus expected IP list"} 1
```toml
dns_query_status_change{agent_hostname="localhost",domain="baidu.com",record_type="A",server="114.114.114.114"} 1
dns_query_status_change_detail{agent_hostname="localhost",diff="182.61.201.211",domain="baidu.com",ips="182.61.201.211,182.61.244.181",record_type="A",server="114.114.114.114"} 1
```

# Alert Rule Configuration
```
Personal experience, for reference only. Typical DNS resolution latency thresholds:
Over 2000 ms: P2 level — send alerts via the WeCom app; if it recovers within 3 minutes, send a recovery notification.
Over 5000 ms: P1 level — trigger voice-call alerts plus WeCom app alerts; if it recovers within 3 minutes, send a recovery notification.

Why design it this way?
When DNS monitoring is in play, company business is usually spread across the country, and different regions can run into all kinds of DNS problems (such as DNS hijacking or regional DNS server failures), so these issues need to be treated with high severity.
The 3-minute window between the alert and the recovery notification is meant to filter out transient issues, while still leaving enough handling time for the SLA (99.99%).
```
