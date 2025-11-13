package clickhouse

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/dskit/sqlbase"
	"github.com/ccfos/nightingale/v6/dskit/types"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/mitchellh/mapstructure"
	"github.com/toolkits/pkg/logger"
	ckDriver "gorm.io/driver/clickhouse"
	"gorm.io/gorm"
)

const (
	ckDataSource = "clickhouse://%s:%s@%s?read_timeout=10s"

	DefaultLimit = 500
)

type Clickhouse struct {
	Nodes        []string `json:"ck.nodes" mapstructure:"ck.nodes"`
	User         string   `json:"ck.user" mapstructure:"ck.user"`
	Password     string   `json:"ck.password" mapstructure:"ck.password"`
	Timeout      int      `json:"ck.timeout" mapstructure:"ck.timeout"`
	MaxQueryRows int      `json:"ck.max_query_rows" mapstructure:"ck.max_query_rows"`
	Protocol     string   `json:"ck.protocol" mapstructure:"ck.protocol"`
	SkipSSL      bool     `json:"ck.skip_ssl" mapstructure:"ck.skip_ssl"`

	// 连接池配置（可选）
	MaxIdleConns    int `json:"ck.max_idle_conns" mapstructure:"ck.max_idle_conns"`       // 最大空闲连接数
	MaxOpenConns    int `json:"ck.max_open_conns" mapstructure:"ck.max_open_conns"`       // 最大打开连接数
	ConnMaxLifetime int `json:"ck.conn_max_lifetime" mapstructure:"ck.conn_max_lifetime"` // 连接最大生命周期（秒）

	Client       *gorm.DB `json:"-"`
	ClientByHTTP *sql.DB  `json:"-"`
}

func (c *Clickhouse) InitCli() error {
	if c.MaxQueryRows == 0 {
		c.MaxQueryRows = DefaultLimit
	}

	if len(c.Nodes) == 0 {
		return fmt.Errorf("not found ck shard, please check datasource config")
	}
	// 前端只允许 host:port，直接使用第一个节点
	addr := c.Nodes[0]

	prot := strings.ToLower(strings.TrimSpace(c.Protocol))
	// 如果用户显式指定 protocol，只允许 http、https 或 native
	if prot != "" {
		if prot != "http" && prot != "https" && prot != "native" {
			return fmt.Errorf("unsupported clickhouse protocol: %s, only `http`, `https` or `native` allowed", c.Protocol)
		}

		// HTTP(S) 路径（使用 clickhouse-go HTTP client）
		if prot == "http" || prot == "https" {
			opts := &clickhouse.Options{
				Addr:        []string{addr},
				Auth:        clickhouse.Auth{Username: c.User, Password: c.Password},
				Settings:    clickhouse.Settings{"max_execution_time": 60},
				DialTimeout: 10 * time.Second,
				Protocol:    clickhouse.HTTP,
			}
			// 仅当显式指定 https 时才启用 TLS 并使用 SkipSSL 控制 InsecureSkipVerify
			if prot == "https" {
				opts.TLS = &tls.Config{InsecureSkipVerify: c.SkipSSL}
			}
			ckconn := clickhouse.OpenDB(opts)
			if ckconn == nil {
				return errors.New("db conn failed")
			}
			// 应用连接池配置到 HTTP sql.DB
			if c.MaxIdleConns > 0 {
				ckconn.SetMaxIdleConns(c.MaxIdleConns)
			}
			if c.MaxOpenConns > 0 {
				ckconn.SetMaxOpenConns(c.MaxOpenConns)
			}
			if c.ConnMaxLifetime > 0 {
				ckconn.SetConnMaxLifetime(time.Duration(c.ConnMaxLifetime) * time.Second)
			}
			c.ClientByHTTP = ckconn
			return nil
		}

		// native 路径（使用 gorm + native driver）
		host := strings.TrimPrefix(strings.TrimPrefix(addr, "http://"), "https://")
		dsn := fmt.Sprintf(ckDataSource, c.User, c.Password, host)
		db, err := gorm.Open(
			ckDriver.New(
				ckDriver.Config{
					DSN:                       dsn,
					DisableDatetimePrecision:  true,
					DontSupportRenameColumn:   true,
					SkipInitializeWithVersion: false,
				}),
		)
		if err != nil {
			return err
		}
		// 应用连接池配置到 gorm 底层 *sql.DB
		if sqlDB, derr := db.DB(); derr == nil {
			if c.MaxIdleConns > 0 {
				sqlDB.SetMaxIdleConns(c.MaxIdleConns)
			}
			if c.MaxOpenConns > 0 {
				sqlDB.SetMaxOpenConns(c.MaxOpenConns)
			}
			if c.ConnMaxLifetime > 0 {
				sqlDB.SetConnMaxLifetime(time.Duration(c.ConnMaxLifetime) * time.Second)
			}
		} else {
			logger.Debugf("clickhouse: get native sql DB failed: %v", derr)
		}
		c.Client = db
		return nil
	}

	// protocol 未指定：直接按 HTTP（host:port -> http）连接，去掉探测逻辑
	opts := &clickhouse.Options{
		Addr:        []string{addr},
		Auth:        clickhouse.Auth{Username: c.User, Password: c.Password},
		Settings:    clickhouse.Settings{"max_execution_time": 60},
		DialTimeout: 10 * time.Second,
		Protocol:    clickhouse.HTTP,
	}
	ckconn := clickhouse.OpenDB(opts)
	if ckconn != nil {
		if c.MaxIdleConns > 0 {
			ckconn.SetMaxIdleConns(c.MaxIdleConns)
		}
		if c.MaxOpenConns > 0 {
			ckconn.SetMaxOpenConns(c.MaxOpenConns)
		}
		if c.ConnMaxLifetime > 0 {
			ckconn.SetConnMaxLifetime(time.Duration(c.ConnMaxLifetime) * time.Second)
		}
		c.ClientByHTTP = ckconn
		return nil
	}

	// 作为最后回退，尝试 native 连接
	host := strings.TrimPrefix(strings.TrimPrefix(addr, "http://"), "https://")
	dsn := fmt.Sprintf(ckDataSource, c.User, c.Password, host)
	db, err := gorm.Open(
		ckDriver.New(
			ckDriver.Config{
				DSN:                       dsn,
				DisableDatetimePrecision:  true,
				DontSupportRenameColumn:   true,
				SkipInitializeWithVersion: false,
			}),
	)
	if err != nil {
		return err
	}
	if sqlDB, derr := db.DB(); derr == nil {
		if c.MaxIdleConns > 0 {
			sqlDB.SetMaxIdleConns(c.MaxIdleConns)
		}
		if c.MaxOpenConns > 0 {
			sqlDB.SetMaxOpenConns(c.MaxOpenConns)
		}
		if c.ConnMaxLifetime > 0 {
			sqlDB.SetConnMaxLifetime(time.Duration(c.ConnMaxLifetime) * time.Second)
		}
	}
	c.Client = db
	return nil
}

const (
	ShowDatabases = "SHOW DATABASES"
	ShowTables    = "SELECT name FROM system.tables WHERE database = '%s'"
	DescTable     = "SELECT name,type FROM system.columns WHERE database='%s' AND table = '%s';"
)

func (c *Clickhouse) QueryRows(ctx context.Context, query string) (*sql.Rows, error) {
	var (
		rows *sql.Rows
		err  error
	)

	if c.ClientByHTTP != nil {
		rows, err = c.ClientByHTTP.Query(query)
		if err != nil {
			return nil, err
		}
	} else if c.Client != nil {
		rows, err = c.Client.Raw(query).Rows()
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("clickhouse client is nil")
	}

	return rows, nil
}

// ShowDatabases lists all databases in Clickhouse
func (c *Clickhouse) ShowDatabases(ctx context.Context) ([]string, error) {
	res := make([]string, 0)

	rows, err := c.QueryRows(ctx, ShowDatabases)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		res = append(res, r)
	}

	return res, nil
}

// ShowTables lists all tables in a given database
func (c *Clickhouse) ShowTables(ctx context.Context, database string) ([]string, error) {
	res := make([]string, 0)

	showTables := fmt.Sprintf(ShowTables, database)
	rows, err := c.QueryRows(ctx, showTables)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		res = append(res, r)
	}

	return res, nil
}

// DescribeTable describes the schema of a specified table in Clickhouse
func (c *Clickhouse) DescribeTable(ctx context.Context, query interface{}) ([]*types.ColumnProperty, error) {
	var (
		ret []*types.ColumnProperty
	)

	ckQueryParam := new(QueryParam)
	if err := mapstructure.Decode(query, ckQueryParam); err != nil {
		return nil, err
	}
	descTable := fmt.Sprintf(DescTable, ckQueryParam.Database, ckQueryParam.Table)

	rows, err := c.QueryRows(ctx, descTable)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var column types.ColumnProperty
		if err := rows.Scan(&column.Field, &column.Type); err != nil {
			return nil, err
		}
		ret = append(ret, &column)
	}

	return ret, nil
}

func (c *Clickhouse) ExecQueryBySqlDB(ctx context.Context, sql string) ([]map[string]interface{}, error) {
	rows, err := c.QueryRows(ctx, sql)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}

	for rows.Next() {
		columnValues := make([]interface{}, len(columns))
		columnPointers := make([]interface{}, len(columns))
		for i := range columnValues {
			columnPointers[i] = &columnValues[i]
		}

		if err := rows.Scan(columnPointers...); err != nil {
			continue
		}

		rowMap := make(map[string]interface{})
		for i, colName := range columns {
			val := columnValues[i]
			bytes, ok := val.([]byte)
			if ok {
				rowMap[colName] = string(bytes)
			} else {
				rowMap[colName] = val
			}
		}
		results = append(results, rowMap)
	}
	return results, nil
}

func (c *Clickhouse) Query(ctx context.Context, query interface{}) ([]map[string]interface{}, error) {

	ckQuery := new(QueryParam)
	if err := mapstructure.Decode(query, ckQuery); err != nil {
		return nil, err
	}

	// 校验SQL的合法性, 过滤掉 write请求
	sqlItem := strings.Split(strings.ToUpper(ckQuery.Sql), " ")
	for _, item := range sqlItem {
		if _, ok := ckBannedOp[item]; ok {
			return nil, fmt.Errorf("operation %s is forbid, only read db, please check your sql", item)
		}
	}

	// 检查匹配数据长度，防止数据量过大
	err := c.CheckMaxQueryRows(ctx, ckQuery.Sql)
	if err != nil {
		return nil, err
	}

	dbRows := make([]map[string]interface{}, 0)

	if c.ClientByHTTP != nil {
		dbRows, err = c.ExecQueryBySqlDB(ctx, ckQuery.Sql)
	} else {
		err = c.Client.Raw(ckQuery.Sql).Find(&dbRows).Error
	}
	if err != nil {
		return nil, fmt.Errorf("fetch data failed, sql is %s, err is %s", ckQuery.Sql, err.Error())
	}

	return dbRows, nil
}

func (c *Clickhouse) CheckMaxQueryRows(ctx context.Context, sql string) error {

	subSql := strings.ReplaceAll(sql, ";", "")
	subSql = fmt.Sprintf("SELECT COUNT(*) as count FROM (%s) AS subquery;", subSql)

	dbRows, err := c.ExecQueryBySqlDB(ctx, subSql)
	if err != nil {
		return fmt.Errorf("fetch data failed, sql is %s, err is %s", subSql, err.Error())
	}

	if len(dbRows) > 0 {
		if count, exists := dbRows[0]["count"]; exists {
			v, err := sqlbase.ParseFloat64Value(count)
			if err != nil {
				return err
			}

			if v > float64(c.MaxQueryRows) {
				return fmt.Errorf("query result rows count %d exceeds the maximum limit %d", int(v), c.MaxQueryRows)
			}
		}
	}

	return nil
}
