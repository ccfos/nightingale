package oracle

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/dskit/pool"
	"github.com/ccfos/nightingale/v6/dskit/sqlbase"
	"github.com/ccfos/nightingale/v6/dskit/types"

	goora "github.com/sijms/go-ora/v2"
	"github.com/mitchellh/mapstructure"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

type Oracle struct {
	Shards []Shard `json:"oracle.shards" mapstructure:"oracle.shards"`
}

type Shard struct {
	Addr            string `json:"oracle.addr" mapstructure:"oracle.addr"`
	DB              string `json:"oracle.db" mapstructure:"oracle.db"`
	User            string `json:"oracle.user" mapstructure:"oracle.user"`
	Password        string `json:"oracle.password" mapstructure:"oracle.password"`
	Timeout         int    `json:"oracle.timeout" mapstructure:"oracle.timeout"`
	MaxIdleConns    int    `json:"oracle.max_idle_conns" mapstructure:"oracle.max_idle_conns"`
	MaxOpenConns    int    `json:"oracle.max_open_conns" mapstructure:"oracle.max_open_conns"`
	ConnMaxLifetime int    `json:"oracle.conn_max_lifetime" mapstructure:"oracle.conn_max_lifetime"`
	MaxQueryRows    int    `json:"oracle.max_query_rows" mapstructure:"oracle.max_query_rows"`
}

type Dialector struct {
	Conn *sql.DB
}

func (d Dialector) Name() string {
	return "oracle"
}

func (d Dialector) Initialize(db *gorm.DB) error {
	db.ConnPool = d.Conn
	return nil
}

func (d Dialector) Migrator(db *gorm.DB) gorm.Migrator {
	return migrator.Migrator{Config: migrator.Config{DB: db}}
}

func (d Dialector) DataTypeOf(field *schema.Field) string {
	switch field.DataType {
	case schema.Bool:
		return "NUMBER(1)"
	case schema.Int:
		return "NUMBER(10)"
	case schema.Uint:
		return "NUMBER(10)"
	case schema.Float:
		return "FLOAT"
	case schema.String:
		return "VARCHAR2(255)"
	case schema.Time:
		return "TIMESTAMP"
	case schema.Bytes:
		return "BLOB"
	default:
		return "VARCHAR2(255)"
	}
}

func (d Dialector) DefaultValueOf(field *schema.Field) clause.Expression {
	return clause.Expr{SQL: "NULL"}
}

func (d Dialector) BindVarTo(writer clause.Writer, stmt *gorm.Statement, v interface{}) {
	writer.WriteString(":")
	writer.WriteString(strconv.Itoa(len(stmt.Vars)))
}

func (d Dialector) QuoteTo(writer clause.Writer, str string) {
	writer.WriteByte('"')
	writer.WriteString(strings.ToUpper(str))
	writer.WriteByte('"')
}

func (d Dialector) Explain(sql string, vars ...interface{}) string {
	return sql
}

func NewOracleWithSettings(ctx context.Context, settings interface{}) (*Oracle, error) {
	newest := new(Oracle)
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

func (o *Oracle) NewConn(ctx context.Context, database string) (*gorm.DB, error) {
	if len(o.Shards) == 0 {
		return nil, errors.New("empty oracle shards")
	}

	shard := o.Shards[0]

	if shard.Timeout == 0 {
		shard.Timeout = 60
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

	var keys []string
	var err error
	keys = append(keys, shard.Addr)
	keys = append(keys, shard.Password, shard.User)
	if len(database) > 0 {
		keys = append(keys, database)
	}
	cachedKey := strings.Join(keys, ":")
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

	dsn := goora.BuildUrl(shard.Addr, 1521, database, shard.User, shard.Password, nil)
	sqlDB, err := sql.Open("oracle", dsn)
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxIdleConns(shard.MaxIdleConns)
	sqlDB.SetMaxOpenConns(shard.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Duration(shard.ConnMaxLifetime) * time.Second)

	if err = sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, err
	}

	db, err = gorm.Open(Dialector{Conn: sqlDB}, &gorm.Config{})
	if err != nil {
		sqlDB.Close()
		return nil, err
	}

	return db.WithContext(ctx), nil
}

func (o *Oracle) ShowDatabases(ctx context.Context) ([]string, error) {
	db, err := o.NewConn(ctx, "")
	if err != nil {
		return nil, err
	}

	return sqlbase.ShowDatabases(ctx, db, "SELECT USERNAME FROM ALL_USERS")
}

func (o *Oracle) ShowTables(ctx context.Context, database string) ([]string, error) {
	db, err := o.NewConn(ctx, database)
	if err != nil {
		return nil, err
	}

	return sqlbase.ShowTables(ctx, db, "SELECT TABLE_NAME FROM ALL_TABLES WHERE OWNER = UPPER('"+database+"')")
}

func (o *Oracle) DescTable(ctx context.Context, database, table string) ([]*types.ColumnProperty, error) {
	if err := sqlbase.ValidateIdentifier(table); err != nil {
		return nil, fmt.Errorf("describe table: %w", err)
	}
	db, err := o.NewConn(ctx, database)
	if err != nil {
		return nil, err
	}

	query := "SELECT COLUMN_NAME, DATA_TYPE, NULLABLE FROM ALL_TAB_COLUMNS WHERE OWNER = UPPER('" + database + "') AND TABLE_NAME = UPPER('" + table + "')"
	return sqlbase.DescTable(ctx, db, query)
}

func (o *Oracle) SelectRows(ctx context.Context, database, table, query string) ([]map[string]interface{}, error) {
	if err := sqlbase.ValidateIdentifier(table); err != nil {
		return nil, fmt.Errorf("select rows: %w", err)
	}
	db, err := o.NewConn(ctx, database)
	if err != nil {
		return nil, err
	}

	sqlStr := "SELECT * FROM " + sqlbase.QuoteDouble(table)
	if query != "" {
		sqlStr += " WHERE " + query
	}
	return sqlbase.ExecQuery(ctx, db, sqlStr)
}

func (o *Oracle) ExecQuery(ctx context.Context, database string, sqlStr string) ([]map[string]interface{}, error) {
	db, err := o.NewConn(ctx, database)
	if err != nil {
		return nil, err
	}

	return sqlbase.ExecQuery(ctx, db, sqlStr)
}
