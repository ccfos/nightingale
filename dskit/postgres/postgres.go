// @Author: Ciusyan 5/20/24

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/dskit/pool"
	"github.com/ccfos/nightingale/v6/dskit/sqlbase"
	"github.com/ccfos/nightingale/v6/dskit/types"

	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/mitchellh/mapstructure"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type PostgreSQL struct {
	Shard `json:",inline"  mapstructure:",squash"`
}

type Shard struct {
	Addr            string `json:"pgsql.addr" mapstructure:"pgsql.addr"`
	DB              string `json:"pgsql.db" mapstructure:"pgsql.db"`
	User            string `json:"pgsql.user" mapstructure:"pgsql.user"`
	Password        string `json:"pgsql.password" mapstructure:"pgsql.password" `
	Timeout         int    `json:"pgsql.timeout" mapstructure:"pgsql.timeout"`
	MaxIdleConns    int    `json:"pgsql.max_idle_conns" mapstructure:"pgsql.max_idle_conns"`
	MaxOpenConns    int    `json:"pgsql.max_open_conns" mapstructure:"pgsql.max_open_conns"`
	ConnMaxLifetime int    `json:"pgsql.conn_max_lifetime" mapstructure:"pgsql.conn_max_lifetime"`
	MaxQueryRows    int    `json:"pgsql.max_query_rows" mapstructure:"pgsql.max_query_rows"`
}

// NewPostgreSQLWithSettings initializes a new PostgreSQL instance with the given settings
func NewPostgreSQLWithSettings(ctx context.Context, settings interface{}) (*PostgreSQL, error) {
	newest := new(PostgreSQL)
	settingsMap := map[string]interface{}{}

	switch s := settings.(type) {
	case string:
		if err := json.Unmarshal([]byte(s), &settingsMap); err != nil {
			return nil, err
		}
	case map[string]interface{}:
		settingsMap = s
	case *PostgreSQL:
		return s, nil
	case PostgreSQL:
		return &s, nil
	case Shard:
		newest.Shard = s
		return newest, nil
	case *Shard:
		newest.Shard = *s
		return newest, nil
	default:
		return nil, errors.New("unsupported settings type")
	}

	if err := mapstructure.Decode(settingsMap, newest); err != nil {
		return nil, err
	}

	return newest, nil
}

// NewConn establishes a new connection to PostgreSQL
func (p *PostgreSQL) NewConn(ctx context.Context, database string) (*gorm.DB, error) {
	if len(p.DB) == 0 && len(database) == 0 {
		return nil, errors.New("empty pgsql database") // 兼容阿里实时数仓Hologres, 连接时必须指定db名字
	}

	if p.Shard.Timeout == 0 {
		p.Shard.Timeout = 60
	}

	if p.Shard.MaxIdleConns == 0 {
		p.Shard.MaxIdleConns = 10
	}

	if p.Shard.MaxOpenConns == 0 {
		p.Shard.MaxOpenConns = 100
	}

	if p.Shard.ConnMaxLifetime == 0 {
		p.Shard.ConnMaxLifetime = 14400
	}

	if len(p.Shard.Addr) == 0 {
		return nil, errors.New("empty fe-node addr")
	}
	var keys []string
	var err error
	keys = append(keys, p.Shard.Addr)

	keys = append(keys, p.Shard.Password, p.Shard.User)
	if len(database) > 0 {
		keys = append(keys, database)
	}
	cachedKey := strings.Join(keys, ":")
	// cache conn with database
	conn, ok := pool.PoolClient.Load(cachedKey)
	if ok {
		return conn.(*gorm.DB), nil
	}

	var db *gorm.DB
	defer func() {
		if db != nil && err == nil {
			pool.PoolClient.Store(cachedKey, db)
		}
	}()

	// Simplified connection logic for PostgreSQL
	dsn := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable&TimeZone=Asia/Shanghai", url.QueryEscape(p.Shard.User), url.QueryEscape(p.Shard.Password), p.Shard.Addr, database)

	db, err = sqlbase.NewDB(
		ctx,
		postgres.Open(dsn),
		p.Shard.MaxIdleConns,
		p.Shard.MaxOpenConns,
		time.Duration(p.Shard.ConnMaxLifetime)*time.Second,
	)

	if err != nil {
		if db != nil {
			sqlDB, _ := db.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}
		}
		return nil, err
	}

	return db, nil
}

// ShowDatabases lists all databases in PostgreSQL
func (p *PostgreSQL) ShowDatabases(ctx context.Context, searchKeyword string) ([]string, error) {
	db, err := p.NewConn(ctx, "postgres")
	if err != nil {
		return nil, err
	}
	sql := fmt.Sprintf("SELECT datname FROM pg_database WHERE datistemplate = false AND datname LIKE %s",
		"'%"+searchKeyword+"%'")
	return sqlbase.ShowDatabases(ctx, db, sql)
}

// ShowTables lists all tables in a given database
func (p *PostgreSQL) ShowTables(ctx context.Context, searchKeyword string) (map[string][]string, error) {
	db, err := p.NewConn(ctx, p.DB)
	if err != nil {
		return nil, err
	}
	sql := fmt.Sprintf("SELECT schemaname, tablename FROM pg_tables WHERE schemaname !='information_schema' and schemaname !='pg_catalog'  and  tablename LIKE %s",
		"'%"+searchKeyword+"%'")
	rets, err := sqlbase.ExecQuery(ctx, db, sql)
	if err != nil {
		return nil, err
	}
	tabs := make(map[string][]string, 3)
	for _, row := range rets {
		if val, ok := row["schemaname"].(string); ok {
			tabs[val] = append(tabs[val], row["tablename"].(string))
		}
	}
	return tabs, nil
}

// DescTable describes the schema of a specified table in PostgreSQL
// scheme default: public if not specified
func (p *PostgreSQL) DescTable(ctx context.Context, scheme, table string) ([]*types.ColumnProperty, error) {
	db, err := p.NewConn(ctx, p.DB)
	if err != nil {
		return nil, err
	}
	if scheme == "" {
		scheme = "public"
	}

	query := fmt.Sprintf("SELECT column_name, data_type, is_nullable, column_default FROM information_schema.columns WHERE table_name = '%s' AND table_schema = '%s'", table, scheme)
	return sqlbase.DescTable(ctx, db, query)
}

// SelectRows selects rows from a specified table in PostgreSQL based on a given query
func (p *PostgreSQL) SelectRows(ctx context.Context, table, where string) ([]map[string]interface{}, error) {
	db, err := p.NewConn(ctx, p.DB)
	if err != nil {
		return nil, err
	}

	return sqlbase.SelectRows(ctx, db, table, where)
}

// ExecQuery executes a SQL query in PostgreSQL
func (p *PostgreSQL) ExecQuery(ctx context.Context, sql string) ([]map[string]interface{}, error) {
	db, err := p.NewConn(ctx, p.DB)
	if err != nil {
		return nil, err
	}

	return sqlbase.ExecQuery(ctx, db, sql)
}
