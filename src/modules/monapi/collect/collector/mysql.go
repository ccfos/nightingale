package collector

import (
	"encoding/json"
	"fmt"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/monapi/collect"
	"github.com/didi/nightingale/src/modules/monapi/collect/collector/mysql"
)

func init() {
	collect.CollectorRegister(&MysqlCollector{})
}

type MysqlCollector struct{}

func (p MysqlCollector) Name() string               { return "mysql" }
func (p MysqlCollector) Category() collect.Category { return collect.RemoteCategory }

func (p MysqlCollector) Template() (interface{}, error) {
	return collect.Template(&MysqlRule{})
}

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
	IntervalSlow                        string   `label:"Interval Slow" json:"interval_slow" desc:"Some queries we may want to run less often (such as SHOW GLOBAL VARIABLES)" example:"interval_slow = '30m'"`
	MetricVersion                       int      `label:"-" json:"-"`
}

func (p MysqlRule) Mysql() *mysql.Mysql {
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
		IntervalSlow:                        p.IntervalSlow,
		MetricVersion:                       2,
	}
}

func (p *MysqlRule) Validate() error {
	if len(p.Servers) == 0 || p.Servers[0] == "" {
		return fmt.Errorf("mysql.rule.servers must be set")
	}
	return nil
}

func (p MysqlCollector) Get(id int64) (interface{}, error) {
	collect := &models.CollectRule{}
	has, err := models.DB["mon"].Where("id = ?", id).Get(collect)
	if !has {
		return nil, err
	}
	return collect, err
}

func (p MysqlCollector) Gets(nids []int64) (ret []interface{}, err error) {
	collects := []models.CollectRule{}
	err = models.DB["mon"].In("nid", nids).Find(&collects)
	for _, c := range collects {
		ret = append(ret, c)
	}
	return ret, err
}

func (p MysqlCollector) GetByNameAndNid(name string, nid int64) (interface{}, error) {
	collect := &models.CollectRule{}
	has, err := models.DB["mon"].Where("collect_type = ? and name = ? and nid = ?", p.Name(), name, nid).Get(collect)
	if !has {
		return nil, err
	}
	return collect, err
}

func (p MysqlCollector) Create(data []byte, username string) error {
	collect := &models.CollectRule{CollectType: p.Name()}
	rule := &MysqlRule{}

	if err := json.Unmarshal(data, collect); err != nil {
		return fmt.Errorf("unmarshal body %s err:%v", string(data), err)
	}

	if err := collect.Validate(rule); err != nil {
		return err
	}

	can, err := models.UsernameCandoNodeOp(username, "mon_collect_create", collect.Nid)
	if err != nil {
		return err
	}
	if !can {
		return fmt.Errorf("permission deny")
	}

	collect.Creator = username
	collect.LastUpdator = username

	old, err := p.GetByNameAndNid(collect.Name, collect.Nid)
	if err != nil {
		return err
	}
	if old != nil {
		return fmt.Errorf("同节点下策略名称 %s 已存在", collect.Name)
	}
	return models.CreateCollect(p.Name(), username, collect)
}

func (p MysqlCollector) Update(data []byte, username string) error {
	collect := &models.CollectRule{}
	rule := &MysqlRule{}

	if err := json.Unmarshal(data, collect); err != nil {
		return fmt.Errorf("unmarshal body %s err:%v", string(data), err)
	}

	if err := collect.Validate(rule); err != nil {
		return err
	}

	can, err := models.UsernameCandoNodeOp(username, "mon_collect_modify", collect.Nid)
	if err != nil {
		return err
	}
	if !can {
		return fmt.Errorf("permission deny")
	}

	//校验采集是否存在
	obj, err := p.Get(collect.Id) //id找不到的情况
	if err != nil {
		return fmt.Errorf("采集不存在 type:%s id:%d", p.Name(), collect.Id)
	}

	tmpId := obj.(*models.CollectRule).Id
	if tmpId == 0 {
		return fmt.Errorf("采集不存在 type:%s id:%d", p.Name(), collect.Id)
	}

	collect.Creator = username
	collect.LastUpdator = username

	old, err := p.GetByNameAndNid(collect.Name, collect.Nid)
	if err != nil {
		return err
	}
	if old != nil && tmpId != old.(*models.CollectRule).Id {
		return fmt.Errorf("同节点下策略名称 %s 已存在", collect.Name)
	}

	return collect.Update()
}

func (p MysqlCollector) Delete(id int64, username string) error {
	tmp, err := p.Get(id) //id找不到的情况
	if err != nil {
		return fmt.Errorf("采集不存在 type:%s id:%d", p.Name(), id)
	}
	nid := tmp.(*models.CollectRule).Nid
	can, err := models.UsernameCandoNodeOp(username, "mon_collect_delete", int64(nid))
	if err != nil {
		return err
	}
	if !can {
		return fmt.Errorf("permission deny")
	}

	return models.DeleteCollectById(p.Name(), username, id)
}
