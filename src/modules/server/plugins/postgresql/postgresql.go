package postgresql

import (
	"fmt"
	"github.com/didi/nightingale/v4/src/common/i18n"
	"github.com/didi/nightingale/v4/src/modules/server/collector"
	"github.com/didi/nightingale/v4/src/modules/server/plugins"
	"github.com/didi/nightingale/v4/src/modules/server/plugins/postgresql/postgresql"
	"github.com/influxdata/telegraf"
	"net"
	"net/url"
	"sort"
	"strings"
)

func init() {
	collector.CollectorRegister(NewPostgresqlCollector())
	i18n.DictRegister(langDict)
}

type PostgresqlCollector struct {
	*collector.BaseCollector
}

var (
	langDict = map[string]map[string]string{
		"zh": map[string]string{
			"Dsn":          "数据库地址",
			"ExcludeDatabases": "不需要监控的数据库",
			"if the list is empty, then metrics are gathered from all database": "如果列表为空，则收集所有数据库表",
			"PgSetting": "数据库全局配置",
			"gather pg setting":"是否采集 pg setting全局配置",
			"StatArchiver": "采集pg_stat_archiver视图",
			"gather pg_stat_archiver":"主要记录WAL归档信息",
			"ReplicationSlot": "采集pg_replication_slot视图",
			"gather pg_replication_slots":"用于确保WAL迁移是否正常",
			"StatReplication":"采集pg_stat_replication视图",
			"gather pg_stat_replication": "pg复制（异步同步）监控",
			"StatDatabaseConfilicts":"采集pg_stat_database_confilicts视图",
			"specify servers via a url matching<br />postgresql://[pqgotest[:password]]@host:port[/dbname]?sslmode=[disable|verify-ca|verify-full]]<br />": "通过URL设置指定服务器<br />postgresql://[pqgotest[:password]]@host:port[/dbname]?sslmode=[disable|verify-ca|verify-full]]<br />",
		},
	}
)

func NewPostgresqlCollector() *PostgresqlCollector {
	return &PostgresqlCollector{BaseCollector: collector.NewBaseCollector(
		"postgresql",
		collector.RemoteCategory,
		func() collector.TelegrafPlugin { return &PostgresqlRule{} },
	)}
}

type PostgresqlRule struct {
	ExcludeDatabases               []string `label:"ExcludeDatabases" json:"exclude_databases" description:"if the list is empty, then metrics are gathered from all database"`
	GatherPgStatReplication        bool     `label:"StatReplication" json:"pg_stat_replication" description:"gather pg_stat_replication" default:"false"`
	GatherPgReplicationSlots       bool     `label:"ReplicationSlot" json:"pg_replication_slots" description:"gather pg_replication_slots" default:"false"`
	GatherPgStatArchiver           bool     `label:"StatArchiver" json:"pg_stat_archiver" description:"gather pg_stat_archiver" default:"false"`
	GatherPgSetting                bool     `label:"PgSetting" json:"pg_setting" description:"gather pg setting" default:"false"`
	Dsn                            string   `label:"Dsn" json:"dsn, required" description:"specify servers via a url matching<br />postgresql://[pqgotest[:password]]@host:port[/dbname]?sslmode=[disable|verify-ca|verify-full]]<br />" example:"postgresql://postgres:xxx@127.0.0.1:5432/postgres?sslmode=disable"`
	plugins.ClientConfig
}

func parseURL(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}

	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", fmt.Errorf("invalid connection protocol: %s", u.Scheme)
	}

	var kvs []string
	escaper := strings.NewReplacer(` `, `\ `, `'`, `\'`, `\`, `\\`)
	accrue := func(k, v string) {
		if v != "" {
			kvs = append(kvs, k+"="+escaper.Replace(v))
		}
	}

	if u.User != nil {
		v := u.User.Username()
		accrue("user", v)

		v, _ = u.User.Password()
		accrue("password", v)
	}

	if host, port, err := net.SplitHostPort(u.Host); err != nil {
		accrue("host", u.Host)
	} else {
		accrue("host", host)
		accrue("port", port)
	}

	if u.Path != "" {
		accrue("dbname", u.Path[1:])
	}

	q := u.Query()
	for k := range q {
		accrue(k, q.Get(k))
	}

	sort.Strings(kvs) // Makes testing easier (not a performance concern)
	return strings.Join(kvs, " "), nil
}

func (p *PostgresqlRule) Validate() error {
	if p.Dsn == "" {
		return fmt.Errorf("postgresql.rule.address must be set")
	}
	_, err := parseURL(p.Dsn)
	if err != nil {
		return fmt.Errorf("address parse failed, detail: %v", err)
	}
	return nil
}
func (p *PostgresqlRule) TelegrafInput() (telegraf.Input, error) {

	if err := p.Validate(); err != nil {
		return nil, err
	}

	return &postgresql.Postgresql{
		Dsn:                            p.Dsn,
		ExcludeDatabases:               p.ExcludeDatabases,
		GatherPgSetting:                p.GatherPgSetting,
		GatherPgStatReplication:        p.GatherPgStatReplication,
		GatherPgReplicationSlots:       p.GatherPgReplicationSlots,
		GatherPgStatArchiver:           p.GatherPgStatArchiver,
	}, nil

}
