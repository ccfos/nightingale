# switch_legacy

A switch monitoring plugin, forked from [https://github.com/gaochao1/swcollector](https://github.com/gaochao1/swcollector). It can automatically detect the network device model and collect CPU and memory usage, as well as monitoring data for each network interface — these are generic OIDs.

## Configuration

The most essential configuration is the IP list, which can be written in three ways:

```toml
[[instances]]
ips = [
    "172.16.2.1",
    "172.16.4/24",
    "192.168.56.102-192.168.56.120"
]
```

This plugin only supports SNMP v2c, so the authentication information is just a community string.

## Unique identity label

Monitoring data from network devices carries an ip label by default, indicating which device the data comes from. If you want to treat the monitoring data as monitored objects in Nightingale so that network devices automatically appear in Nightingale's monitored objects table, simply set switch_id_label to ident. The device's IP will then be reported as the value of the ident label, and Nightingale will automatically read the ident label value and store it.

## Name mapping

Sometimes, seeing only the IP of a network device, we cannot tell which device it actually is. In that case you can map the IP to a name:

```ini
[mappings]
"192.168.88.160" = "switch001.bj"
"192.168.88.161" = "switch002.bj"
```

This way, the reported monitoring data is identified by a string like switch001.bj instead of the IP, which is more readable.

## Custom OIDs

Multiple `[[instances.customs]]` sections can be configured to define custom OIDs. By default, this plugin collects monitoring data for each network interface of the device, as well as CPU and memory usage. If you want to collect other OIDs, use this custom feature.
