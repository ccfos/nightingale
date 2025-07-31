package postgresql

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/pkg/macros"

	"github.com/ccfos/nightingale/v6/dskit/postgres"
	"github.com/ccfos/nightingale/v6/dskit/sqlbase"
	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/mitchellh/mapstructure"
	"github.com/toolkits/pkg/logger"
)

const (
	PostgreSQLType = "pgsql"
)

var (
	regx = `(?i)from\s+((?:"[^"]+"|[a-zA-Z0-9_]+))\.((?:"[^"]+"|[a-zA-Z0-9_]+))\.((?:"[^"]+"|[a-zA-Z0-9_]+))`
)

func init() {
	datasource.RegisterDatasource(PostgreSQLType, new(PostgreSQL))
}

type PostgreSQL struct {
	Shards []*postgres.PostgreSQL `json:"pgsql.shards" mapstructure:"pgsql.shards"`
}

type QueryParam struct {
	Ref      string          `json:"ref" mapstructure:"ref"`
	Database string          `json:"database" mapstructure:"database"`
	Table    string          `json:"table" mapstructure:"table"`
	SQL      string          `json:"sql" mapstructure:"sql"`
	Keys     datasource.Keys `json:"keys" mapstructure:"keys"`
	From     int64           `json:"from" mapstructure:"from"`
	To       int64           `json:"to" mapstructure:"to"`
}

func (p *PostgreSQL) InitClient() error {
	if len(p.Shards) == 0 {
		return fmt.Errorf("not found postgresql addr, please check datasource config")
	}
	for _, shard := range p.Shards {
		if db, err := shard.NewConn(context.TODO(), "postgres"); err != nil {
			defer sqlbase.CloseDB(db)
			return err
		}
	}
	return nil
}

func (p *PostgreSQL) Init(settings map[string]interface{}) (datasource.Datasource, error) {
	newest := new(PostgreSQL)
	err := mapstructure.Decode(settings, newest)
	return newest, err
}

func (p *PostgreSQL) Validate(ctx context.Context) error {
	if len(p.Shards) == 0 || len(strings.TrimSpace(p.Shards[0].Addr)) == 0 {
		return fmt.Errorf("postgresql addr is invalid, please check datasource setting")
	}

	if len(strings.TrimSpace(p.Shards[0].User)) == 0 {
		return fmt.Errorf("postgresql user is invalid, please check datasource setting")
	}

	return nil
}

// Equal compares whether two objects are the same, used for caching
func (p *PostgreSQL) Equal(d datasource.Datasource) bool {
	newest, ok := d.(*PostgreSQL)
	if !ok {
		logger.Errorf("unexpected plugin type, expected is postgresql")
		return false
	}

	if len(p.Shards) == 0 || len(newest.Shards) == 0 {
		return false
	}

	oldShard := p.Shards[0]
	newShard := newest.Shards[0]

	if oldShard.Addr != newShard.Addr {
		return false
	}

	if oldShard.User != newShard.User {
		return false
	}

	if oldShard.Password != newShard.Password {
		return false
	}

	if oldShard.MaxQueryRows != newShard.MaxQueryRows {
		return false
	}

	if oldShard.Timeout != newShard.Timeout {
		return false
	}

	if oldShard.MaxIdleConns != newShard.MaxIdleConns {
		return false
	}

	if oldShard.MaxOpenConns != newShard.MaxOpenConns {
		return false
	}

	if oldShard.ConnMaxLifetime != newShard.ConnMaxLifetime {
		return false
	}

	return true
}

func (p *PostgreSQL) ShowDatabases(ctx context.Context) ([]string, error) {
	return p.Shards[0].ShowDatabases(ctx, "")
}

func (p *PostgreSQL) ShowTables(ctx context.Context, database string) ([]string, error) {
	p.Shards[0].DB = database
	rets, err := p.Shards[0].ShowTables(ctx, "")
	if err != nil {
		return nil, err
	}
	tables := make([]string, 0, len(rets))
	for scheme, tabs := range rets {
		for _, tab := range tabs {
			tables = append(tables, scheme+"."+tab)
		}
	}
	return tables, nil
}

func (p *PostgreSQL) MakeLogQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return nil, nil
}

func (p *PostgreSQL) MakeTSQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return nil, nil
}

func (p *PostgreSQL) QueryMapData(ctx context.Context, query interface{}) ([]map[string]string, error) {
	return nil, nil
}

func (p *PostgreSQL) QueryData(ctx context.Context, query interface{}) ([]models.DataResp, error) {
	postgresqlQueryParam := new(QueryParam)
	if err := mapstructure.Decode(query, postgresqlQueryParam); err != nil {
		return nil, err
	}

	postgresqlQueryParam.SQL = formatSQLDatabaseNameWithRegex(postgresqlQueryParam.SQL)
	if strings.Contains(postgresqlQueryParam.SQL, "$__") {
		var err error
		postgresqlQueryParam.SQL, err = macros.Macro(postgresqlQueryParam.SQL, postgresqlQueryParam.From, postgresqlQueryParam.To)
		if err != nil {
			return nil, err
		}
	}
	if postgresqlQueryParam.Database != "" {
		p.Shards[0].DB = postgresqlQueryParam.Database
	} else {
		db, err := parseDBName(postgresqlQueryParam.SQL)
		if err != nil {
			return nil, err
		}
		p.Shards[0].DB = db
	}

	timeout := p.Shards[0].Timeout
	if timeout == 0 {
		timeout = 60
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	items, err := p.Shards[0].QueryTimeseries(timeoutCtx, &sqlbase.QueryParam{
		Sql: postgresqlQueryParam.SQL,
		Keys: types.Keys{
			ValueKey: postgresqlQueryParam.Keys.ValueKey,
			LabelKey: postgresqlQueryParam.Keys.LabelKey,
			TimeKey:  postgresqlQueryParam.Keys.TimeKey,
		},
	})

	if err != nil {
		logger.Warningf("query:%+v get data err:%v", postgresqlQueryParam, err)
		return []models.DataResp{}, err
	}
	data := make([]models.DataResp, 0)
	for i := range items {
		data = append(data, models.DataResp{
			Ref:    postgresqlQueryParam.Ref,
			Metric: items[i].Metric,
			Values: items[i].Values,
		})
	}

	// parse resp to time series data
	logger.Infof("req:%+v keys:%+v \n data:%v", postgresqlQueryParam, postgresqlQueryParam.Keys, data)

	return data, nil
}

func (p *PostgreSQL) QueryLog(ctx context.Context, query interface{}) ([]interface{}, int64, error) {
	postgresqlQueryParam := new(QueryParam)
	if err := mapstructure.Decode(query, postgresqlQueryParam); err != nil {
		return nil, 0, err
	}
	if postgresqlQueryParam.Database != "" {
		p.Shards[0].DB = postgresqlQueryParam.Database
	} else {
		db, err := parseDBName(postgresqlQueryParam.SQL)
		if err != nil {
			return nil, 0, err
		}
		p.Shards[0].DB = db
	}

	postgresqlQueryParam.SQL = formatSQLDatabaseNameWithRegex(postgresqlQueryParam.SQL)
	if strings.Contains(postgresqlQueryParam.SQL, "$__") {
		var err error
		postgresqlQueryParam.SQL, err = macros.Macro(postgresqlQueryParam.SQL, postgresqlQueryParam.From, postgresqlQueryParam.To)
		if err != nil {
			return nil, 0, err
		}
	}

	timeout := p.Shards[0].Timeout
	if timeout == 0 {
		timeout = 60
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	items, err := p.Shards[0].Query(timeoutCtx, &sqlbase.QueryParam{
		Sql: postgresqlQueryParam.SQL,
	})
	if err != nil {
		logger.Warningf("query:%+v get data err:%v", postgresqlQueryParam, err)
		return []interface{}{}, 0, err
	}
	logs := make([]interface{}, 0)
	for i := range items {
		logs = append(logs, items[i])
	}

	return logs, 0, nil
}

func (p *PostgreSQL) DescribeTable(ctx context.Context, query interface{}) ([]*types.ColumnProperty, error) {
	postgresqlQueryParam := new(QueryParam)
	if err := mapstructure.Decode(query, postgresqlQueryParam); err != nil {
		return nil, err
	}
	p.Shards[0].DB = postgresqlQueryParam.Database
	pairs := strings.Split(postgresqlQueryParam.Table, ".") // format: scheme.table_name
	scheme := ""
	table := postgresqlQueryParam.Table
	if len(pairs) == 2 {
		scheme = pairs[0]
		table = pairs[1]
	}
	return p.Shards[0].DescTable(ctx, scheme, table)
}

func parseDBName(sql string) (db string, err error) {
	re := regexp.MustCompile(regx)
	matches := re.FindStringSubmatch(sql)
	if len(matches) != 4 {
		return "", fmt.Errorf("no valid table name in format database.schema.table found")
	}
	return strings.Trim(matches[1], `"`), nil
}

// formatSQLDatabaseNameWithRegex 只对 dbname.scheme.tabname 格式进行数据库名称格式化，转为 "dbname".scheme.tabname
// 在pgsql中，大小写是通过"" 双引号括起来区分的,默认pg都是转为小写的，所以这里转为 "dbname".scheme."tabname"
func formatSQLDatabaseNameWithRegex(sql string) string {
	// 匹配 from dbname.scheme.table_name 的模式
	// 使用捕获组来精确匹配数据库名称，确保后面跟着scheme和table
	re := regexp.MustCompile(`(?i)\bfrom\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\.\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\.\s*([a-zA-Z_][a-zA-Z0-9_]*)`)

	return re.ReplaceAllString(sql, `from "$1"."$2"."$3"`)
}

func extractColumns(sql string) ([]string, error) {
	// 将 SQL 转换为小写以简化匹配
	sql = strings.ToLower(sql)

	// 匹配 SELECT 和 FROM 之间的内容
	re := regexp.MustCompile(`select\s+(.*?)\s+from`)
	matches := re.FindStringSubmatch(sql)

	if len(matches) < 2 {
		return nil, fmt.Errorf("no columns found or invalid SQL syntax")
	}

	// 提取列部分
	columnsString := matches[1]

	// 分割列
	columns := splitColumns(columnsString)

	// 清理每个列名
	for i, col := range columns {
		columns[i] = strings.TrimSpace(col)
	}

	return columns, nil
}

func splitColumns(columnsString string) []string {
	var columns []string
	var currentColumn strings.Builder
	parenthesesCount := 0
	inQuotes := false

	for _, char := range columnsString {
		switch char {
		case '(':
			parenthesesCount++
			currentColumn.WriteRune(char)
		case ')':
			parenthesesCount--
			currentColumn.WriteRune(char)
		case '\'', '"':
			inQuotes = !inQuotes
			currentColumn.WriteRune(char)
		case ',':
			if parenthesesCount == 0 && !inQuotes {
				columns = append(columns, currentColumn.String())
				currentColumn.Reset()
			} else {
				currentColumn.WriteRune(char)
			}
		default:
			currentColumn.WriteRune(char)
		}
	}

	if currentColumn.Len() > 0 {
		columns = append(columns, currentColumn.String())
	}

	return columns
}
