# netstat_filter

This plugin collects network connection information and filters and aggregates it based on user-defined conditions, so you can monitor exactly the connections you care about.
## Metrics
tcp_established  
tcp_syn_sent
tcp_syn_recv
tcp_fin_wait1
tcp_fin_wait2
tcp_time_wait
tcp_close
tcp_close_wait
tcp_last_ack
tcp_listen
tcp_closing
tcp_none
tcp_send_queue
tcp_recv_queue

## Feature Description
After filtering by source IP, source port, destination IP and destination port, the plugin collects the recv-Q and send-Q of the matched connections. These metrics reflect the quality of the specified connections very well. For example, if the RTT is too long and ACKs from the server arrive slowly, send-Q will stay above 0 for a long time; monitoring can catch this in time so that you can optimize the network or the application in advance.

When the filter matches multiple connections, the send and recv values are summed.
For example:
with ``raddr_port = 11883`` in the configuration file,
if connections to port 11883 exist both locally and with several different IPs, the results of these connections are summed. Likewise, with many concurrent connections, the values are merged and summed — in short, the coarser the filter, the more connections get summed together.

To define multiple rules, duplicate the ``[[instances]]`` section.

## Notes
The netstat_filter_tcp_send_queue and netstat_filter_tcp_recv_queue metrics are currently only supported on Linux. On Windows they default to 0.
