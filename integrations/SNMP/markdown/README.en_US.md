# snmp

> Network devices are monitored mainly via the SNMP protocol. Categraf, Telegraf, Datadog-Agent and snmp_exporter all provide this capability.

Starting from v0.2.13, Categraf has integrated Telegraf's snmp plugin, and we recommend using it to monitor network devices. The core logic of this plugin is: to collect a metric, simply configure the corresponding OID; data collected from some OIDs can even be used as labels of the time series, which makes it extremely flexible.

Of course, there are downsides too. The SNMP world contains a large number of private OIDs — for example, the OIDs for CPU and memory utilization differ across devices — so different device models require different configurations, which is tedious to maintain and requires a lot of accumulated knowledge. We encourage everyone to contribute collection configurations for different device models to [this repository](https://github.com/flashcatcloud/categraf/tree/main/inputs/snmp), one folder per model. Accumulated over time, this will benefit both you and others. If you are not sure how to submit a PR, feel free to contact us.

That said, there is no need to be too pessimistic: for network devices, most monitoring data can be collected with generic OIDs. For example:

```toml
interval = 120

[[instances]]
agents = ["udp://172.30.15.189:161"]

interval_times = 1
timeout = "5s"
version = 2
community = "public"
agent_host_tag = "switch_ip"
retries = 1

[[instances.field]]
oid = "RFC1213-MIB::sysUpTime.0"
name = "uptime"

[[instances.field]]
oid = "RFC1213-MIB::sysName.0"
name = "source"
is_tag = true

[[instances.table]]
oid = "IF-MIB::ifTable"
name = "interface"
inherit_tags = ["source"]

[[instances.table.field]]
oid = "IF-MIB::ifDescr"
name = "ifDescr"
is_tag = true

```

The sample above uses SNMP v2. For v3, an example of the authentication settings:

```toml
version = 3
sec_name = "managev3user"
auth_protocol = "SHA"
auth_password = "example.Demo.c0m"
```

In addition, for SNMP collection we recommend deploying a dedicated Categraf, because different monitored objects may need different collection frequencies. For example, collecting from edge switches every 5 minutes is enough, while core switches can be collected more frequently, e.g. every 60s or 120s.

> Note: if you collect too frequently, some older switches may be overwhelmed or may throttle the requests; being throttled shows up as gaps in the charts.

## Further Reading

- [Introduction to SNMP (Simple Network Management Protocol)](https://flashcat.cloud/blog/snmp-introduction/)
- [SNMP Command Arguments Explained](https://flashcat.cloud/blog/snmp-command-arguments/)
- [Collecting Monitoring Data with the Categraf SNMP Plugin](https://flashcat.cloud/blog/snmp-metrics-collect-by-categraf/)

## Troubleshooting

To collect SNMP data with categraf, first make sure the machine running categraf can reach the network device. You can test with the snmpget command:

```bash
snmpget -v2c -c public 172.30.15.189 RFC1213-MIB::sysUpTime.0
```

If even snmpget does not work, fix that first — possible causes include snmpd not running, a firewall blocking SNMP access, or the snmpget command not being installed, etc. GPT and Google can help you solve these issues, so we won't go into detail here.
