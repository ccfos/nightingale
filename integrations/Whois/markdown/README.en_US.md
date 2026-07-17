# whois

Domain probing plugin, used to probe the registration time and expiration time of a domain. The values are UTC+0 timestamps.


## Configuration

The most important setting is the domain option, which specifies the target to probe. For example, to monitor a domain:
It is commented out by default; while commented out, the plugin is disabled.

```toml
# [[instances]]
## Used to collect domain name information.
# domain = "baidu.com"
```
Note that this should be a domain name, not a URL.

## Metric Descriptions

whois_domain_createddate domain creation timestamp
whois_domain_updateddate domain update timestamp
whois_domain_expirationdate domain expiration timestamp

## Notes
Do not set the interval too short, as it would cause frequent request timeouts. There is little need for it, so please keep the collection interval as long as possible.
