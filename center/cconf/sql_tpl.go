package cconf

var TDengineSQLTpl = map[string]string{
	"load5":                "SELECT _wstart as ts, last(load5) FROM $database.system WHERE host = '$server' and _ts >= $from and _ts <= $to interval($interval) fill(null)",
	"process_total":        "SELECT _wstart as ts, last(total) FROM $database.processes WHERE host = '$server' and _ts >= $from and _ts <= $to interval($interval) fill(null)",
	"thread_total":         "SELECT _wstart as ts, last(total) FROM $database.threads WHERE host = '$server' and _ts >= $from and _ts <= $to interval($interval) fill(null)",
	"cpu_idle":             "SELECT _wstart as ts, last(usage_idle) * -1 + 100 FROM $database.cpu WHERE (host = '$server' and cpu = 'cpu-total') and _ts >= $from and _ts <= $to interval($interval) fill(null)",
	"mem_used_percent":     "SELECT _wstart as ts, last(used_percent) FROM $database.mem WHERE (host = '$server') and _ts >= $from and _ts <= $to interval($interval) fill(null)",
	"disk_used_percent":    "SELECT _wstart as ts, last(used_percent) FROM $database.disk WHERE (host = '$server' and path = '/') and _ts >= $from and _ts <= $to interval($interval) fill(null)",
	"cpu_context_switches": "select ts, derivative(context_switches, 1s, 0) as context FROM (SELECT _wstart as ts, avg(context_switches) as context_switches FROM $database.kernel WHERE host = '$server' and _ts >= $from and _ts <= $to interval($interval) )",
	"tcp":                  "SELECT _wstart as ts, avg(tcp_close) as CLOSED, avg(tcp_close_wait) as CLOSE_WAIT, avg(tcp_closing) as CLOSING, avg(tcp_established) as ESTABLISHED, avg(tcp_fin_wait1) as FIN_WAIT1, avg(tcp_fin_wait2) as FIN_WAIT2, avg(tcp_last_ack) as LAST_ACK, avg(tcp_syn_recv) as SYN_RECV, avg(tcp_syn_sent) as SYN_SENT, avg(tcp_time_wait) as TIME_WAIT FROM $database.netstat WHERE host = '$server' and _ts >= $from and _ts <= $to interval($interval)",
	"net_bytes_recv":       "SELECT _wstart as ts, derivative(bytes_recv,1s, 1) as bytes_in FROM $database.net WHERE host = '$server' and interface = '$netif' and _ts >= $from and _ts <= $to group by tbname",
	"net_bytes_sent":       "SELECT _wstart as ts, derivative(bytes_sent,1s, 1) as bytes_out FROM $database.net WHERE host = '$server' and interface = '$netif' and _ts >= $from and _ts <= $to group by tbname",
	"disk_total":           "SELECT _wstart as ts, avg(total) AS total, avg(used) as used FROM $database.disk WHERE   path = '$mountpoint' and _ts >= $from and _ts <= $to  interval($interval) group by host",
}
