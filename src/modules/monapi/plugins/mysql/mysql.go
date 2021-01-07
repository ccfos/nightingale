package mysql

import (
	"fmt"

	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/didi/nightingale/src/toolkits/i18n"
	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs/mysql"
)

func init() {
	collector.CollectorRegister(NewMysqlCollector()) // for monapi
	i18n.DictRegister(langDict)
}

type MysqlCollector struct {
	*collector.BaseCollector
}

func NewMysqlCollector() *MysqlCollector {
	return &MysqlCollector{BaseCollector: collector.NewBaseCollector(
		"mysql",
		collector.RemoteCategory,
		func() interface{} { return &MysqlRule{} },
	)}
}

var (
	langDict = map[string]map[string]string{
		"zh": map[string]string{
			"Servers":   "服务",
			"Databases": "数据库",
			"if the list is empty, then metrics are gathered from all database tables": "如果列表为空，则收集所有数据库表",
			"Process List": "进程列表",
			"gather thread state counts from INFORMATION_SCHEMA.PROCESSLIST": "从 INFORMATION_SCHEMA.PROCESSLIST 收集线程状态信息",
			"User Statistics": "User Statistics",
			"gather user statistics from INFORMATION_SCHEMA.USER_STATISTICS": "从 INFORMATION_SCHEMA.USER_STATISTICS 收集用户状态信息",
			"Auto Increment": "Auto Increment",
			"gather auto_increment columns and max values from information schema": "采集 auto_increment 和 max values 信息",
			"Innodb Metrics": "Innodb Metrics",
			"gather metrics from INFORMATION_SCHEMA.INNODB_METRICS": "采集 INFORMATION_SCHEMA.INNODB_METRICS 信息",
			"Slave Status": "Slave Status",
			"gather metrics from SHOW SLAVE STATUS command output": "采集 SHOW SLAVE STATUS command output",
			"Binary Logs": "Binary Logs",
			"gather metrics from SHOW BINARY LOGS command output": "采集 SHOW BINARY LOGS command output",
			"Table IO Waits": "Table IO Waits",
			"gather metrics from PERFORMANCE_SCHEMA.TABLE_IO_WAITS_SUMMARY_BY_TABLE": "采集 PERFORMANCE_SCHEMA.TABLE_IO_WAITS_SUMMARY_BY_TABLE",
			"Table Lock Waits": "Table Lock Waits",
			"gather metrics from PERFORMANCE_SCHEMA.TABLE_LOCK_WAITS": "采集 PERFORMANCE_SCHEMA.TABLE_LOCK_WAITS",
			"Index IO Waits": "Index IO Waits",
			"gather metrics from PERFORMANCE_SCHEMA.TABLE_IO_WAITS_SUMMARY_BY_INDEX_USAGE": "采集 PERFORMANCE_SCHEMA.TABLE_IO_WAITS_SUMMARY_BY_INDEX_USAGE",
			"Event Waits": "Event Waits",
			"gather metrics from PERFORMANCE_SCHEMA.EVENT_WAITS": "采集 PERFORMANCE_SCHEMA.EVENT_WAITS",
			"Tables": "Tables",
			"gather metrics from INFORMATION_SCHEMA.TABLES for databases provided above list": "采集 INFORMATION_SCHEMA.TABLES for databases provided above list",
			"File Events Stats": "File Events Stats",
			"gather metrics from PERFORMANCE_SCHEMA.FILE_SUMMARY_BY_EVENT_NAME": "采集 PERFORMANCE_SCHEMA.FILE_SUMMARY_BY_EVENT_NAME",
			"Perf Events Statements": "Perf Events Statements",
			"gather metrics from PERFORMANCE_SCHEMA.EVENTS_STATEMENTS_SUMMARY_BY_DIGEST": "采集 PERFORMANCE_SCHEMA.EVENTS_STATEMENTS_SUMMARY_BY_DIGEST",
			"Interval Slow": "Interval Slow",
		},
	}
)

type MysqlRule struct {
	Servers                             []string `label:"Servers" json:"servers,required" description:"specify servers via a url matching\n[username[:password]@][protocol[(address)]]/[?tls=[true|false|skip-verify|custom]]\nsee https://github.com/go-sql-driver/mysql#dsn-data-source-name" example:"servers = ['user:passwd@tcp(127.0.0.1:3306)/?tls=false']\nservers = ["user@tcp(127.0.0.1:3306)/?tls=false"]"`
	PerfEventsStatementsDigestTextLimit int64    `label:"-" json:"-"`
	PerfEventsStatementsLimit           int64    `label:"-" json:"-"`
	PerfEventsStatementsTimeLimit       int64    `label:"-" json:"-"`
	TableSchemaDatabases                []string `label:"Databases" json:"table_schema_databases" description:"if the list is empty, then metrics are gathered from all database tables"`
	GatherProcessList                   bool     `label:"Process List" json:"gather_process_list" description:"gather thread state counts from INFORMATION_SCHEMA.PROCESSLIST"`
	GatherUserStatistics                bool     `label:"User Statistics" json:"gather_user_statistics" description:"gather user statistics from INFORMATION_SCHEMA.USER_STATISTICS"`
	GatherInfoSchemaAutoInc             bool     `label:"Auto Increment" json:"gather_info_schema_auto_inc" description:"gather auto_increment columns and max values from information schema"`
	GatherInnoDBMetrics                 bool     `label:"Innodb Metrics" json:"gather_innodb_metrics" description:"gather metrics from INFORMATION_SCHEMA.INNODB_METRICS"`
	GatherSlaveStatus                   bool     `label:"Slave Status" json:"gather_slave_status" description:"gather metrics from SHOW SLAVE STATUS command output"`
	GatherBinaryLogs                    bool     `label:"Binary Logs" json:"gather_binary_logs" description:"gather metrics from SHOW BINARY LOGS command output"`
	GatherTableIOWaits                  bool     `label:"Table IO Waits" json:"gather_table_io_waits" description:"gather metrics from PERFORMANCE_SCHEMA.TABLE_IO_WAITS_SUMMARY_BY_TABLE"`
	GatherTableLockWaits                bool     `label:"Table Lock Waits" json:"gather_table_lock_waits" description:"gather metrics from PERFORMANCE_SCHEMA.TABLE_LOCK_WAITS"`
	GatherIndexIOWaits                  bool     `label:"Index IO Waits" json:"gather_index_io_waits" description:"gather metrics from PERFORMANCE_SCHEMA.TABLE_IO_WAITS_SUMMARY_BY_INDEX_USAGE"`
	GatherEventWaits                    bool     `label:"Event Waits" json:"gather_event_waits" description:"gather metrics from PERFORMANCE_SCHEMA.EVENT_WAITS"`
	GatherTableSchema                   bool     `label:"Tables" json:"gather_table_schema" description:"gather metrics from INFORMATION_SCHEMA.TABLES for databases provided above list"`
	GatherFileEventsStats               bool     `label:"File Events Stats" json:"gather_file_events_stats" description:"gather metrics from PERFORMANCE_SCHEMA.FILE_SUMMARY_BY_EVENT_NAME"`
	GatherPerfEventsStatements          bool     `label:"Perf Events Statements" json:"gather_perf_events_statements" description:"gather metrics from PERFORMANCE_SCHEMA.EVENTS_STATEMENTS_SUMMARY_BY_DIGEST"`
	GatherGlobalVars                    bool     `label:"-" json:"-"`
	IntervalSlow                        string   `label:"Interval Slow" json:"interval_slow" desc:"Some queries we may want to run less often (such as SHOW GLOBAL VARIABLES)" example:"interval_slow = '30m'" json:"-"`
	MetricVersion                       int      `label:"-" json:"-"`
}

func (p *MysqlRule) Validate() error {
	if len(p.Servers) == 0 || p.Servers[0] == "" {
		return fmt.Errorf("mysql.rule.servers must be set")
	}
	return nil
}

func (p *MysqlRule) TelegrafInput() (telegraf.Input, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	return &mysql.Mysql{
		Servers:                             p.Servers,
		PerfEventsStatementsDigestTextLimit: 120,
		PerfEventsStatementsLimit:           250,
		PerfEventsStatementsTimeLimit:       86400,
		TableSchemaDatabases:                p.TableSchemaDatabases,
		GatherProcessList:                   p.GatherProcessList,
		GatherUserStatistics:                p.GatherUserStatistics,
		GatherInfoSchemaAutoInc:             p.GatherInfoSchemaAutoInc,
		GatherInnoDBMetrics:                 p.GatherInnoDBMetrics,
		GatherSlaveStatus:                   p.GatherSlaveStatus,
		GatherBinaryLogs:                    p.GatherBinaryLogs,
		GatherTableIOWaits:                  p.GatherTableIOWaits,
		GatherTableLockWaits:                p.GatherTableLockWaits,
		GatherIndexIOWaits:                  p.GatherIndexIOWaits,
		GatherEventWaits:                    p.GatherEventWaits,
		GatherTableSchema:                   p.GatherTableSchema,
		GatherFileEventsStats:               p.GatherFileEventsStats,
		GatherPerfEventsStatements:          p.GatherPerfEventsStatements,
		GatherGlobalVars:                    true,
		IntervalSlow:                        "0m",
		MetricVersion:                       2,
	}, nil
}
