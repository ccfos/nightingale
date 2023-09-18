package cconf

var TDengineSQLTpl = map[string]string{
	"getDatabases":     "show databases",
	"getTables":        "show dbName.tables",
	"query cpu_ide":    "SELECT _wstart as ts, last(usage_idle) * -1 + 100 FROM $database.cpu WHERE _ts >= $from and _ts <= $to  interval($interval) group by _wstart order by _wstart desc",
	"query cpu_usage":  "SELECT _wstart as ts, last(usage_idle) * -1 + 100 FROM $database.cpu WHERE _ts >= $from and _ts <= $to  interval($interval) group by _wstart order by _wstart desc",
	"query cpu_guest":  "SELECT _wstart as ts, last(usage_idle) * -1 + 100 FROM $database.cpu WHERE _ts >= $from and _ts <= $to  interval($interval) group by _wstart order by _wstart desc",
	"query cpu_ide4":   "SELECT _wstart as ts, last(usage_idle) * -1 + 100 FROM $database.cpu WHERE _ts >= $from and _ts <= $to  interval($interval) group by _wstart order by _wstart desc",
	"query cpu_id5":    "SELECT _wstart as ts, last(usage_idle) * -1 + 100 FROM $database.cpu WHERE _ts >= $from and _ts <= $to  interval($interval) group by _wstart order by _wstart desc",
	"query cpu_ide6":   "SELECT _wstart as ts, last(usage_idle) * -1 + 100 FROM $database.cpu WHERE _ts >= $from and _ts <= $to  interval($interval) group by _wstart order by _wstart desc",
	"query cpu_ide7":   "SELECT _wstart as ts, last(usage_idle) * -1 + 100 FROM $database.cpu WHERE _ts >= $from and _ts <= $to  interval($interval) group by _wstart order by _wstart desc",
	"query cpu_ide8":   "SELECT _wstart as ts, last(usage_idle) * -1 + 100 FROM $database.cpu WHERE _ts >= $from and _ts <= $to  interval($interval) group by _wstart order by _wstart desc",
	"query cpu_ide9":   "SELECT _wstart as ts, last(usage_idle) * -1 + 100 FROM $database.cpu WHERE _ts >= $from and _ts <= $to  interval($interval) group by _wstart order by _wstart desc",
	"query mem use":    "SELECT _wstart as ts, last(usage_idle) * -1 + 100 FROM $database.cpu WHERE _ts >= $from and _ts <= $to  interval($interval) group by _wstart order by _wstart desc",
	"query mem free":   "SELECT _wstart as ts, last(usage_idle) * -1 + 100 FROM $database.cpu WHERE _ts >= $from and _ts <= $to  interval($interval) group by _wstart order by _wstart desc",
	"query mem total":  "SELECT _wstart as ts, last(usage_idle) * -1 + 100 FROM $database.cpu WHERE _ts >= $from and _ts <= $to  interval($interval) group by _wstart order by _wstart desc",
	"query disk total": "SELECT _wstart as ts, last(usage_idle) * -1 + 100 FROM $database.cpu WHERE _ts >= $from and _ts <= $to  interval($interval) group by _wstart order by _wstart desc",
	"query disk use":   "SELECT _wstart as ts, last(usage_idle) * -1 + 100 FROM $database.cpu WHERE _ts >= $from and _ts <= $to  interval($interval) group by _wstart order by _wstart desc",
	"query disk free":  "SELECT _wstart as ts, last(usage_idle) * -1 + 100 FROM $database.cpu WHERE _ts >= $from and _ts <= $to  interval($interval) group by _wstart order by _wstart desc",
	"query io util":    "SELECT _wstart as ts, last(usage_idle) * -1 + 100 FROM $database.cpu WHERE _ts >= $from and _ts <= $to  interval($interval) group by _wstart order by _wstart desc",
}
