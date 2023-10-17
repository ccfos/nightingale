# Consul Input Plugin

This plugin will collect statistics about all health checks registered in the
Consul. It uses [Consul API][1] to query the data. It will not report the
[telemetry][2] but Consul can report those stats already using StatsD protocol
if needed.

[1]: https://www.consul.io/docs/agent/http/health.html#health_state

[2]: https://www.consul.io/docs/agent/telemetry.html

## Global configuration options

In addition to the plugin-specific configuration settings, plugins support
additional global and plugin configuration settings. These settings are used to
modify metrics, tags, and field or create aliases and configure ordering, etc.
See the [README.md][README.md] for more details.

[README.md]: ../README.md

## Configuration

```toml
# Gather health check statuses from services registered in Consul
[[instances]]
  ## Consul server address
  # address = "localhost:8500"

  ## URI scheme for the Consul server, one of "http", "https"
  # scheme = "http"

  ## ACL token used in every request
  # token = ""

  ## HTTP Basic Authentication username and password.
  # username = ""
  # password = ""

  ## Data center to query the health checks from
  # datacenter = ""

  ## Optional TLS Config
  # tls_ca = "/etc/categraf/ca.pem"
  # tls_cert = "/etc/categraf/cert.pem"
  # tls_key = "/etc/categraf/key.pem"
  ## Use TLS but skip chain & host verification
  # insecure_skip_verify = true
```

## Metrics

| name                          | help                                                                                                  |
| ----------------------------- | ----------------------------------------------------------------------------------------------------- |
| consul_up                     | Was the last query of Consul successful.                                                              |
| consul_scrape_use_seconds     | scrape use seconds.                                                                                   |
| consul_raft_peers             | How many peers (servers) are in the Raft cluster.                                                     |
| consul_raft_leader            | Does Raft cluster have a leader (according to this node).                                             |
| consul_serf_lan_members       | How many members are in the cluster.                                                                  |
| consul_serf_lan_member_status | Status of member in the cluster. 1=Alive, 2=Leaving, 3=Left, 4=Failed.                                |
| consul_serf_wan_member_status | Status of member in the wan cluster. 1=Alive, 2=Leaving, 3=Left, 4=Failed.                            |
| consul_catalog_services       | How many services are in the cluster.                                                                 |
| consul_service_tag            | Tags of a service.                                                                                    |
| consul_health_node_status     | Status of health checks associated with a node.                                                       |
| consul_health_service_status  | Status of health checks associated with a service.                                                    |
| consul_service_checks         | Link the service id and check name if available.                                                      |
| consul_catalog_kv             | The values for selected keys in Consul's key/value catalog. Keys with non-numeric values are omitted. |
And some metrics with uncertain names, See the [Agent Metrics][Agent Metrics] for more details

[Agent Metrics]: https://developer.hashicorp.com/consul/api-docs/agent#view-metrics

## Example Output

```text
consul_up address=localhost:8500 agent_hostname=hostname 1

consul_scrape_use_seconds address=localhost:8500 agent_hostname=hostname 0.015674053

consul_raft_peers address=localhost:8500 agent_hostname=hostname 1

consul_raft_leader address=localhost:8500 agent_hostname=hostname 1

consul_serf_lan_members address=localhost:8500 agent_hostname=hostname 1

consul_serf_lan_member_status address=localhost:8500 agent_hostname=hostname member=localhost.localdomain 1

consul_serf_wan_member_status address=localhost:8500 agent_hostname=hostname dc=dc1 member=localhost.localdomain.dc1 1

consul_catalog_services address=localhost:8500 agent_hostname=hostname 1

consul_health_node_status address=localhost:8500 agent_hostname=hostname check_id=service:demo check_name=Service 'demo' check node=localhost.localdomain status=passing 1
consul_health_node_status address=localhost:8500 agent_hostname=hostname check_id=service:demo check_name=Service 'demo' check node=localhost.localdomain status=warning 0
consul_health_node_status address=localhost:8500 agent_hostname=hostname check_id=service:demo check_name=Service 'demo' check node=localhost.localdomain status=critical 0
consul_health_node_status address=localhost:8500 agent_hostname=hostname check_id=service:demo check_name=Service 'demo' check node=localhost.localdomain status=maintenance 0

consul_health_service_status address=localhost:8500 agent_hostname=hostname check_id=service:demo check_name=Service 'demo' check node=localhost.localdomain service_id=demo service_name=demo status=passing 1
consul_health_service_status address=localhost:8500 agent_hostname=hostname check_id=service:demo check_name=Service 'demo' check node=localhost.localdomain service_id=demo service_name=demo status=warning 0
consul_health_service_status address=localhost:8500 agent_hostname=hostname check_id=service:demo check_name=Service 'demo' check node=localhost.localdomain service_id=demo service_name=demo status=critical 0
consul_health_service_status address=localhost:8500 agent_hostname=hostname check_id=service:demo check_name=Service 'demo' check node=localhost.localdomain service_id=demo service_name=demo status=maintenance 0

consul_service_checks address=localhost:8500 agent_hostname=hostname check_id=service:demo check_name=Service 'demo' check node=localhost.localdomain service_id=demo service_name=demo status=critical 1

consul_service_tag address=localhost:8500 agent_hostname=hostname check_id=service:demo check_name=Service 'demo' check node=localhost.localdomain service_id=demo service_name=demo tag=tag1 1
consul_service_tag address=localhost:8500 agent_hostname=hostname check_id=service:demo check_name=Service 'demo' check node=localhost.localdomain service_id=demo service_name=demo tag=tag2 1

```
