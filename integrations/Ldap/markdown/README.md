# LDAP Input Plugin

This plugin gathers metrics from LDAP servers' monitoring (`cn=Monitor`)
backend. Currently this plugin supports [OpenLDAP](https://www.openldap.org/)
and [389ds](https://www.port389.org/) servers.

To use this plugin you must enable the monitoring backend/plugin of your LDAP
server. See
[OpenLDAP](https://www.openldap.org/devel/admin/monitoringslapd.html) or 389ds
documentation for details.

## Metrics

Depending on the server dialect, different metrics are produced. The metrics
are usually named according to the selected dialect.

### Tags

- server -- Server name or IP
- port   -- Port used for connecting

## Example Output

Using the `openldap` dialect

```text
openldap_modify_operations_completed agent_hostname=zy-fat port=389 server=localhost 0
openldap_referrals_statistics agent_hostname=zy-fat port=389 server=localhost 0
openldap_unbind_operations_initiated agent_hostname=zy-fat port=389 server=localhost 0
openldap_delete_operations_completed agent_hostname=zy-fat port=389 server=localhost 0
openldap_extended_operations_completed agent_hostname=zy-fat port=389 server=localhost 0
openldap_pdu_statistics agent_hostname=zy-fat port=389 server=localhost 42
openldap_starting_threads agent_hostname=zy-fat port=389 server=localhost 0
openldap_active_threads agent_hostname=zy-fat port=389 server=localhost 1
openldap_uptime_time agent_hostname=zy-fat port=389 server=localhost 102
openldap_bytes_statistics agent_hostname=zy-fat port=389 server=localhost 3176
openldap_compare_operations_completed agent_hostname=zy-fat port=389 server=localhost 0
openldap_bind_operations_completed agent_hostname=zy-fat port=389 server=localhost 1
openldap_total_connections agent_hostname=zy-fat port=389 server=localhost 1002
openldap_search_operations_completed agent_hostname=zy-fat port=389 server=localhost 1
openldap_abandon_operations_initiated agent_hostname=zy-fat port=389 server=localhost 0
openldap_add_operations_initiated agent_hostname=zy-fat port=389 server=localhost 0
openldap_open_threads agent_hostname=zy-fat port=389 server=localhost 1
openldap_add_operations_completed agent_hostname=zy-fat port=389 server=localhost 0
openldap_operations_initiated agent_hostname=zy-fat port=389 server=localhost 3
openldap_write_waiters agent_hostname=zy-fat port=389 server=localhost 0
openldap_entries_statistics agent_hostname=zy-fat port=389 server=localhost 41
openldap_modrdn_operations_completed agent_hostname=zy-fat port=389 server=localhost 0
openldap_pending_threads agent_hostname=zy-fat port=389 server=localhost 0
openldap_max_pending_threads agent_hostname=zy-fat port=389 server=localhost 0
openldap_bind_operations_initiated agent_hostname=zy-fat port=389 server=localhost 1
openldap_max_file_descriptors_connections agent_hostname=zy-fat port=389 server=localhost 1024
openldap_compare_operations_initiated agent_hostname=zy-fat port=389 server=localhost 0
openldap_search_operations_initiated agent_hostname=zy-fat port=389 server=localhost 2
openldap_modrdn_operations_initiated agent_hostname=zy-fat port=389 server=localhost 0
openldap_read_waiters agent_hostname=zy-fat port=389 server=localhost 1
openldap_backload_threads agent_hostname=zy-fat port=389 server=localhost 1
openldap_current_connections agent_hostname=zy-fat port=389 server=localhost 1
openldap_unbind_operations_completed agent_hostname=zy-fat port=389 server=localhost 0
openldap_delete_operations_initiated agent_hostname=zy-fat port=389 server=localhost 0
openldap_extended_operations_initiated agent_hostname=zy-fat port=389 server=localhost 0
openldap_modify_operations_initiated agent_hostname=zy-fat port=389 server=localhost 0
openldap_max_threads agent_hostname=zy-fat port=389 server=localhost 16
openldap_abandon_operations_completed agent_hostname=zy-fat port=389 server=localhost 0
openldap_operations_completed agent_hostname=zy-fat port=389 server=localhost 2
openldap_database_2_databases agent_hostname=zy-fat port=389 server=localhost 0
```

Using the `389ds` dialect

```text
389ds_current_connections_at_max_threads agent_hostname=zy-fat port=389 server=localhost 0
389ds_connections_max_threads agent_hostname=zy-fat port=389 server=localhost 0
389ds_add_operations agent_hostname=zy-fat port=389 server=localhost 0
389ds_dtablesize agent_hostname=zy-fat port=389 server=localhost 63936
389ds_strongauth_binds agent_hostname=zy-fat port=389 server=localhost 13
389ds_modrdn_operations agent_hostname=zy-fat port=389 server=localhost 0
389ds_maxthreads_per_conn_hits agent_hostname=zy-fat port=389 server=localhost 0
389ds_current_connections agent_hostname=zy-fat port=389 server=localhost 2
389ds_security_errors agent_hostname=zy-fat port=389 server=localhost 0
389ds_entries_sent agent_hostname=zy-fat port=389 server=localhost 13
389ds_cache_entries agent_hostname=zy-fat port=389 server=localhost 0
389ds_backends agent_hostname=zy-fat port=389 server=localhost 0
389ds_threads agent_hostname=zy-fat port=389 server=localhost 17
389ds_connections agent_hostname=zy-fat port=389 server=localhost 2
389ds_read_operations agent_hostname=zy-fat port=389 server=localhost 0
389ds_entries_returned agent_hostname=zy-fat port=389 server=localhost 13
389ds_unauth_binds agent_hostname=zy-fat port=389 server=localhost 0
389ds_search_operations agent_hostname=zy-fat port=389 server=localhost 14
389ds_simpleauth_binds agent_hostname=zy-fat port=389 server=localhost 0
389ds_operations_completed agent_hostname=zy-fat port=389 server=localhost 51
389ds_connections_in_max_threads agent_hostname=zy-fat port=389 server=localhost 0
389ds_modify_operations agent_hostname=zy-fat port=389 server=localhost 0
389ds_wholesubtree_search_operations agent_hostname=zy-fat port=389 server=localhost 1
389ds_read_waiters agent_hostname=zy-fat port=389 server=localhost 0
389ds_compare_operations agent_hostname=zy-fat port=389 server=localhost 0
389ds_errors agent_hostname=zy-fat port=389 server=localhost 13
389ds_in_operations agent_hostname=zy-fat port=389 server=localhost 52
389ds_total_connections agent_hostname=zy-fat port=389 server=localhost 15
389ds_cache_hits agent_hostname=zy-fat port=389 server=localhost 0
389ds_list_operations agent_hostname=zy-fat port=389 server=localhost 0
389ds_referrals_returned agent_hostname=zy-fat port=389 server=localhost 0
389ds_copy_entries agent_hostname=zy-fat port=389 server=localhost 0
389ds_operations_initiated agent_hostname=zy-fat port=389 server=localhost 52
389ds_chainings agent_hostname=zy-fat port=389 server=localhost 0
389ds_bind_security_errors agent_hostname=zy-fat port=389 server=localhost 0
389ds_onelevel_search_operations agent_hostname=zy-fat port=389 server=localhost 0
389ds_bytes_sent agent_hostname=zy-fat port=389 server=localhost 1702
389ds_bytes_received agent_hostname=zy-fat port=389 server=localhost 0
389ds_referrals agent_hostname=zy-fat port=389 server=localhost 0
389ds_delete_operations agent_hostname=zy-fat port=389 server=localhost 0
389ds_anonymous_binds agent_hostname=zy-fat port=389 server=localhost 0
```