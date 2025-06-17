// @Author: Ciusyan 5/10/24

package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/dskit/pool"
	"github.com/ccfos/nightingale/v6/dskit/sqlbase"
	"github.com/ccfos/nightingale/v6/dskit/types"

	_ "github.com/go-sql-driver/mysql" // MySQL driver
	"github.com/mitchellh/mapstructure"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type MySQL struct {
	Shards []Shard `json:"mysql.shards" mapstructure:"mysql.shards"`
}

type Shard struct {
	Addr            string `json:"mysql.addr" mapstructure:"mysql.addr"`
	DB              string `json:"mysql.db" mapstructure:"mysql.db"`
	User            string `json:"mysql.user" mapstructure:"mysql.user"`
	Password        string `json:"mysql.password" mapstructure:"mysql.password"`
	Timeout         int    `json:"mysql.timeout" mapstructure:"mysql.timeout"`
	MaxIdleConns    int    `json:"mysql.max_idle_conns" mapstructure:"mysql.max_idle_conns"`
	MaxOpenConns    int    `json:"mysql.max_open_conns" mapstructure:"mysql.max_open_conns"`
	ConnMaxLifetime int    `json:"mysql.conn_max_lifetime" mapstructure:"mysql.conn_max_lifetime"`
	MaxQueryRows    int    `json:"mysql.max_query_rows" mapstructure:"mysql.max_query_rows"`
}

func NewMySQLWithSettings(ctx context.Context, settings interface{}) (*MySQL, error) {
	newest := new(MySQL)
	settingsMap := map[string]interface{}{}

	switch s := settings.(type) {
	case string:
		if err := json.Unmarshal([]byte(s), &settingsMap); err != nil {
			return nil, err
		}
	case map[string]interface{}:
		settingsMap = s
	default:
		return nil, errors.New("unsupported settings type")
	}

	if err := mapstructure.Decode(settingsMap, newest); err != nil {
		return nil, err
	}

	return newest, nil
}

// NewConn establishes a new connection to MySQL
func (m *MySQL) NewConn(ctx context.Context, database string) (*gorm.DB, error) {
	if len(m.Shards) == 0 {
		return nil, errors.New("empty pgsql shards")
	}

	shard := m.Shards[0]

	if shard.Timeout == 0 {
		shard.Timeout = 300
	}

	if shard.MaxIdleConns == 0 {
		shard.MaxIdleConns = 10
	}

	if shard.MaxOpenConns == 0 {
		shard.MaxOpenConns = 100
	}

	if shard.ConnMaxLifetime == 0 {
		shard.ConnMaxLifetime = 300
	}

	if shard.MaxQueryRows == 0 {
		shard.MaxQueryRows = 100
	}

	if len(shard.Addr) == 0 {
		return nil, errors.New("empty addr")
	}

	if len(shard.Addr) == 0 {
		return nil, errors.New("empty addr")
	}
	var keys []string
	var err error
	keys = append(keys, shard.Addr)

	keys = append(keys, shard.Password, shard.User)
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

	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8&parseTime=True", shard.User, shard.Password, shard.Addr, database)

	return sqlbase.NewDB(
		ctx,
		mysql.Open(dsn),
		shard.MaxIdleConns,
		shard.MaxOpenConns,
		time.Duration(shard.ConnMaxLifetime)*time.Second,
	)
}

func (m *MySQL) ShowDatabases(ctx context.Context) ([]string, error) {
	db, err := m.NewConn(ctx, "")
	if err != nil {
		return nil, err
	}

	return sqlbase.ShowDatabases(ctx, db, "SHOW DATABASES")
}

func (m *MySQL) ShowTables(ctx context.Context, database string) ([]string, error) {
	db, err := m.NewConn(ctx, database)
	if err != nil {
		return nil, err
	}

	return sqlbase.ShowTables(ctx, db, "SHOW TABLES")
}

func (m *MySQL) DescTable(ctx context.Context, database, table string) ([]*types.ColumnProperty, error) {
	db, err := m.NewConn(ctx, database)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf("DESCRIBE %s", table)
	return sqlbase.DescTable(ctx, db, query)
}

func (m *MySQL) SelectRows(ctx context.Context, database, table, query string) ([]map[string]interface{}, error) {
	db, err := m.NewConn(ctx, database)
	if err != nil {
		return nil, err
	}

	return sqlbase.SelectRows(ctx, db, table, query)
}

func (m *MySQL) ExecQuery(ctx context.Context, database string, sql string) ([]map[string]interface{}, error) {
	db, err := m.NewConn(ctx, database)
	if err != nil {
		return nil, err
	}

	return sqlbase.ExecQuery(ctx, db, sql)
}
