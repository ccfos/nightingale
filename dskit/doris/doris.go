package doris

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"

	"github.com/ccfos/nightingale/v6/dskit/pool"
	"github.com/ccfos/nightingale/v6/dskit/types"

	_ "github.com/go-sql-driver/mysql" // MySQL driver
	"github.com/mitchellh/mapstructure"
)

const (
	ShowIndexFieldIndexType  = "index_type"
	ShowIndexFieldColumnName = "column_name"
	ShowIndexKeyName         = "key_name"

	SQLShowIndex = "SHOW INDEX FROM "
)

// Doris struct to hold connection details and the connection object
type Doris struct {
	Addr            string `json:"doris.addr" mapstructure:"doris.addr"`         // fe mysql endpoint
	FeAddr          string `json:"doris.fe_addr" mapstructure:"doris.fe_addr"`   // fe http endpoint
	User            string `json:"doris.user" mapstructure:"doris.user"`         //
	Password        string `json:"doris.password" mapstructure:"doris.password"` //
	Timeout         int    `json:"doris.timeout" mapstructure:"doris.timeout"`   // ms
	MaxIdleConns    int    `json:"doris.max_idle_conns" mapstructure:"doris.max_idle_conns"`
	MaxOpenConns    int    `json:"doris.max_open_conns" mapstructure:"doris.max_open_conns"`
	ConnMaxLifetime int    `json:"doris.conn_max_lifetime" mapstructure:"doris.conn_max_lifetime"`
	MaxQueryRows    int    `json:"doris.max_query_rows" mapstructure:"doris.max_query_rows"`
	ClusterName     string `json:"doris.cluster_name" mapstructure:"doris.cluster_name"`
	EnableWrite     bool   `json:"doris.enable_write" mapstructure:"doris.enable_write"`
	// 写用户，用来区分读写用户，减少数据源
	UserWrite     string `json:"doris.user_write" mapstructure:"doris.user_write"`
	PasswordWrite string `json:"doris.password_write" mapstructure:"doris.password_write"`
}

// NewDorisWithSettings initializes a new Doris instance with the given settings
func NewDorisWithSettings(ctx context.Context, settings interface{}) (*Doris, error) {
	newest := new(Doris)
	settingsMap := map[string]interface{}{}
	if reflect.TypeOf(settings).Kind() == reflect.String {
		if err := json.Unmarshal([]byte(settings.(string)), &settingsMap); err != nil {
			return nil, err
		}
	} else {
		var assert bool
		settingsMap, assert = settings.(map[string]interface{})
		if !assert {
			return nil, errors.New("settings type invalid")
		}
	}
	if err := mapstructure.Decode(settingsMap, newest); err != nil {
		return nil, err
	}

	return newest, nil
}

// NewConn establishes a new connection to Doris
func (d *Doris) NewConn(ctx context.Context, database string) (*sql.DB, error) {
	if len(d.Addr) == 0 {
		return nil, errors.New("empty fe-node addr")
	}

	// Set default values similar to postgres implementation
	if d.Timeout == 0 {
		d.Timeout = 60000
	}
	if d.MaxIdleConns == 0 {
		d.MaxIdleConns = 10
	}
	if d.MaxOpenConns == 0 {
		d.MaxOpenConns = 100
	}
	if d.ConnMaxLifetime == 0 {
		d.ConnMaxLifetime = 14400
	}
	if d.MaxQueryRows == 0 {
		d.MaxQueryRows = 500
	}

	var keys []string
	keys = append(keys, d.Addr)
	keys = append(keys, d.User, d.Password)
	if len(database) > 0 {
		keys = append(keys, database)
	}
	cachedKey := strings.Join(keys, ":")
	// cache conn with database
	conn, ok := pool.PoolClient.Load(cachedKey)
	if ok {
		return conn.(*sql.DB), nil
	}
	var db *sql.DB
	var err error
	defer func() {
		if db != nil && err == nil {
			pool.PoolClient.Store(cachedKey, db)
		}
	}()

	// Simplified connection logic for Doris using MySQL driver
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8", d.User, d.Password, d.Addr, database)
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	// Set connection pool configuration
	db.SetMaxIdleConns(d.MaxIdleConns)
	db.SetMaxOpenConns(d.MaxOpenConns)
	db.SetConnMaxLifetime(time.Duration(d.ConnMaxLifetime) * time.Second)

	return db, nil
}

// NewWriteConn establishes a new connection to Doris for write operations
// When EnableWrite is true and UserWrite is configured, it uses the write user credentials
// Otherwise, it reuses the read connection from NewConn
func (d *Doris) NewWriteConn(ctx context.Context, database string) (*sql.DB, error) {
	// If write user is not configured, reuse the read connection
	if !d.EnableWrite || len(d.UserWrite) == 0 {
		return d.NewConn(ctx, database)
	}

	if len(d.Addr) == 0 {
		return nil, errors.New("empty fe-node addr")
	}

	// Set default values similar to postgres implementation
	if d.Timeout == 0 {
		d.Timeout = 60000
	}
	if d.MaxIdleConns == 0 {
		d.MaxIdleConns = 10
	}
	if d.MaxOpenConns == 0 {
		d.MaxOpenConns = 100
	}
	if d.ConnMaxLifetime == 0 {
		d.ConnMaxLifetime = 14400
	}
	if d.MaxQueryRows == 0 {
		d.MaxQueryRows = 500
	}

	// Use write user credentials
	user := d.UserWrite
	password := d.PasswordWrite

	var keys []string
	keys = append(keys, d.Addr)
	keys = append(keys, user, password)
	if len(database) > 0 {
		keys = append(keys, database)
	}
	cachedKey := strings.Join(keys, ":")
	// cache conn with database
	conn, ok := pool.PoolClient.Load(cachedKey)
	if ok {
		return conn.(*sql.DB), nil
	}
	var db *sql.DB
	var err error
	defer func() {
		if db != nil && err == nil {
			pool.PoolClient.Store(cachedKey, db)
		}
	}()

	// Simplified connection logic for Doris using MySQL driver
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8", user, password, d.Addr, database)
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	// Set connection pool configuration for write connections
	// Use more conservative values since write operations are typically less frequent
	writeMaxIdleConns := max(d.MaxIdleConns/5, 2)
	writeMaxOpenConns := max(d.MaxOpenConns/10, 5)

	db.SetMaxIdleConns(writeMaxIdleConns)
	db.SetMaxOpenConns(writeMaxOpenConns)
	db.SetConnMaxLifetime(time.Duration(d.ConnMaxLifetime) * time.Second)

	return db, nil
}

// createTimeoutContext creates a context with timeout based on Doris configuration
func (d *Doris) createTimeoutContext(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := d.Timeout
	if timeout == 0 {
		timeout = 60
	}
	return context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
}

// ShowDatabases lists all databases in Doris
func (d *Doris) ShowDatabases(ctx context.Context) ([]string, error) {
	timeoutCtx, cancel := d.createTimeoutContext(ctx)
	defer cancel()

	db, err := d.NewConn(timeoutCtx, "")
	if err != nil {
		return []string{}, err
	}

	rows, err := db.QueryContext(timeoutCtx, "SHOW DATABASES")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	databases := make([]string, 0)
	for rows.Next() {
		var dbName string
		if err := rows.Scan(&dbName); err != nil {
			continue
		}
		databases = append(databases, dbName)
	}
	return databases, nil
}

// ShowResources lists all resources with type resourceType in Doris
func (d *Doris) ShowResources(ctx context.Context, resourceType string) ([]string, error) {
	timeoutCtx, cancel := d.createTimeoutContext(ctx)
	defer cancel()

	db, err := d.NewConn(timeoutCtx, "")
	if err != nil {
		return []string{}, err
	}

	// 使用 SHOW RESOURCES 命令
	query := fmt.Sprintf("SHOW RESOURCES WHERE RESOURCETYPE = '%s'", resourceType)
	rows, err := db.QueryContext(timeoutCtx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	distinctName := make(map[string]struct{})

	// 获取列信息
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// 准备接收数据的变量
	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	// 遍历结果集
	for rows.Next() {
		err := rows.Scan(valuePtrs...)
		if err != nil {
			return nil, fmt.Errorf("error scanning row: %w", err)
		}
		// 提取资源名称并添加到 map 中（自动去重）
		if name, ok := values[0].([]byte); ok {
			distinctName[string(name)] = struct{}{}
		} else if nameStr, ok := values[0].(string); ok {
			distinctName[nameStr] = struct{}{}
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	// 将 map 转换为切片
	resources := make([]string, 0)
	for name := range distinctName {
		resources = append(resources, name)
	}

	return resources, nil
}

// ShowTables lists all tables in a given database
func (d *Doris) ShowTables(ctx context.Context, database string) ([]string, error) {
	timeoutCtx, cancel := d.createTimeoutContext(ctx)
	defer cancel()

	db, err := d.NewConn(timeoutCtx, database)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf("SHOW TABLES IN %s", database)
	rows, err := db.QueryContext(timeoutCtx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tables := make([]string, 0)
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			continue
		}
		tables = append(tables, tableName)
	}
	return tables, nil
}

// DescTable describes the schema of a specified table in Doris
func (d *Doris) DescTable(ctx context.Context, database, table string) ([]*types.ColumnProperty, error) {
	timeoutCtx, cancel := d.createTimeoutContext(ctx)
	defer cancel()

	db, err := d.NewConn(timeoutCtx, database)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf("DESCRIBE %s.%s", database, table)
	rows, err := db.QueryContext(timeoutCtx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 日志报表中需要把 .type 转化成内部类型
	// TODO: 是否有复合类型, Array/JSON/Tuple/Nested, 是否有更多的类型
	convertDorisType := func(origin string) (string, bool) {
		lower := strings.ToLower(origin)
		switch lower {
		case "double":
			return types.LogExtractValueTypeFloat, true

		case "datetime", "date":
			return types.LogExtractValueTypeDate, false

		case "text":
			return types.LogExtractValueTypeText, true

		default:
			if strings.Contains(lower, "int") {
				return types.LogExtractValueTypeLong, true
			}
			// 日期类型统一按照.date处理
			if strings.HasPrefix(lower, "date") {
				return types.LogExtractValueTypeDate, false
			}
			if strings.HasPrefix(lower, "varchar") || strings.HasPrefix(lower, "char") {
				return types.LogExtractValueTypeText, true
			}
			if strings.HasPrefix(lower, "decimal") {
				return types.LogExtractValueTypeFloat, true
			}
		}

		return origin, false
	}

	var columns []*types.ColumnProperty
	for rows.Next() {
		var (
			field        string
			typ          string
			null         string
			key          string
			defaultValue sql.NullString
			extra        string
		)
		if err := rows.Scan(&field, &typ, &null, &key, &defaultValue, &extra); err != nil {
			continue
		}
		type2, indexable := convertDorisType(typ)
		columns = append(columns, &types.ColumnProperty{
			Field: field,
			Type:  typ, // You might want to convert MySQL types to your custom types

			Type2:     type2,
			Indexable: indexable,
		})
	}
	return columns, nil
}

type TableIndexInfo struct {
	ColumnName string `json:"column_name"`
	IndexName  string `json:"index_name"`
	IndexType  string `json:"index_type"`
}

// ShowIndexes 查询表的所有索引信息
func (d *Doris) ShowIndexes(ctx context.Context, database, table string) ([]TableIndexInfo, error) {
	if database == "" || table == "" {
		return nil, fmt.Errorf("database and table names cannot be empty")
	}

	tCtx, cancel := d.createTimeoutContext(ctx)
	defer cancel()

	db, err := d.NewConn(tCtx, database)
	if err != nil {
		return nil, err
	}

	querySQL := fmt.Sprintf("%s `%s`.`%s`", SQLShowIndex, database, table)
	rows, err := db.QueryContext(tCtx, querySQL)
	if err != nil {
		return nil, fmt.Errorf("failed to query indexes: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}
	count := len(columns)

	// 预映射列索引
	colIdx := map[string]int{
		ShowIndexKeyName:         -1,
		ShowIndexFieldColumnName: -1,
		ShowIndexFieldIndexType:  -1,
	}
	for i, col := range columns {
		lCol := strings.ToLower(col)
		if lCol == ShowIndexKeyName || lCol == ShowIndexFieldColumnName || lCol == ShowIndexFieldIndexType {
			colIdx[lCol] = i
		}
	}

	var result []TableIndexInfo
	for rows.Next() {
		// 使用 sql.RawBytes 可以接受任何类型并转为 string，避免复杂的类型断言
		scanArgs := make([]interface{}, count)
		values := make([]sql.RawBytes, count)
		for i := range values {
			scanArgs[i] = &values[i]
		}

		if err = rows.Scan(scanArgs...); err != nil {
			return nil, err
		}

		info := TableIndexInfo{}
		if i := colIdx[ShowIndexFieldColumnName]; i != -1 && i < count {
			info.ColumnName = string(values[i])
		}
		if i := colIdx[ShowIndexKeyName]; i != -1 && i < count {
			info.IndexName = string(values[i])
		}
		if i := colIdx[ShowIndexFieldIndexType]; i != -1 && i < count {
			info.IndexType = string(values[i])
		}

		if info.ColumnName != "" {
			result = append(result, info)
		}
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return result, nil
}

// SelectRows selects rows from a specified table in Doris based on a given query with MaxQueryRows check
func (d *Doris) SelectRows(ctx context.Context, database, table, query string) ([]map[string]interface{}, error) {
	sql := fmt.Sprintf("SELECT * FROM %s.%s", database, table)
	if query != "" {
		sql += " " + query
	}

	// 检查查询结果行数
	err := d.CheckMaxQueryRows(ctx, database, sql)
	if err != nil {
		return nil, err
	}

	return d.ExecQuery(ctx, database, sql)
}

// ExecQuery executes a given SQL query in Doris and returns the results
func (d *Doris) ExecQuery(ctx context.Context, database string, sql string) ([]map[string]interface{}, error) {
	timeoutCtx, cancel := d.createTimeoutContext(ctx)
	defer cancel()

	db, err := d.NewConn(timeoutCtx, database)
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(timeoutCtx, sql)
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

// ExecContext executes a given SQL query in Doris and returns the results
func (d *Doris) ExecContext(ctx context.Context, database string, sql string) error {
	timeoutCtx, cancel := d.createTimeoutContext(ctx)
	defer cancel()

	db, err := d.NewWriteConn(timeoutCtx, database)
	if err != nil {
		return err
	}

	_, err = db.ExecContext(timeoutCtx, sql)
	return err
}

// ExecBatchSQL 执行多条 SQL 语句
func (d *Doris) ExecBatchSQL(ctx context.Context, database string, sqlBatch string) error {
	// 分割 SQL 语句
	sqlStatements := SplitSQLStatements(sqlBatch)

	// 逐条执行 SQL 语句
	for _, ql := range sqlStatements {
		// 跳过空语句
		ql = strings.TrimSpace(ql)
		if ql == "" {
			continue
		}

		// 检查是否是 CREATE DATABASE 语句
		isCreateDB := strings.HasPrefix(strings.ToUpper(ql), "CREATE DATABASE")
		// strings.HasPrefix(strings.ToUpper(sql), "CREATE SCHEMA") // 暂时不支持CREATE SCHEMA

		// 对于 CREATE DATABASE 语句，使用空数据库名连接
		currentDB := database
		if isCreateDB {
			currentDB = ""
		}

		// 执行单条 SQL，ExecContext 内部已经包含超时处理
		err := d.ExecContext(ctx, currentDB, ql)
		if err != nil {
			return fmt.Errorf("exec sql failed, sql:%s, err:%w", sqlBatch, err)
		}
	}

	return nil
}

// SplitSQLStatements 将多条 SQL 语句分割成单独的语句
func SplitSQLStatements(sqlBatch string) []string {
	var statements []string
	var currentStatement strings.Builder

	// 状态标记
	var (
		inString           bool // 是否在字符串内
		inComment          bool // 是否在单行注释内
		inMultilineComment bool // 是否在多行注释内
		escaped            bool // 前一个字符是否为转义字符
	)

	for i := 0; i < len(sqlBatch); i++ {
		char := sqlBatch[i]
		currentStatement.WriteByte(char)

		// 处理转义字符
		if inString && char == '\\' {
			escaped = !escaped
			continue
		}

		// 处理字符串
		if char == '\'' && !inComment && !inMultilineComment {
			if !escaped {
				inString = !inString
			}
			escaped = false
			continue
		}

		// 处理单行注释
		if !inString && !inMultilineComment && !inComment && char == '-' && i+1 < len(sqlBatch) && sqlBatch[i+1] == '-' {
			inComment = true
			currentStatement.WriteByte(sqlBatch[i+1]) // 写入第二个'-'
			i++
			continue
		}

		// 处理多行注释开始
		if !inString && !inComment && char == '/' && i+1 < len(sqlBatch) && sqlBatch[i+1] == '*' {
			inMultilineComment = true
			currentStatement.WriteByte(sqlBatch[i+1]) // 写入'*'
			i++
			continue
		}

		// 处理多行注释结束
		if inMultilineComment && char == '*' && i+1 < len(sqlBatch) && sqlBatch[i+1] == '/' {
			inMultilineComment = false
			currentStatement.WriteByte(sqlBatch[i+1]) // 写入'/'
			i++
			continue
		}

		// 处理换行符，结束单行注释
		if inComment && (char == '\n' || char == '\r') {
			inComment = false
		}

		// 分割SQL语句
		if char == ';' && !inString && !inMultilineComment && !inComment {
			// 收集到分号后面的单行注释（如果有）
			for j := i + 1; j < len(sqlBatch); j++ {
				nextChar := sqlBatch[j]

				// 检查是否是注释开始
				if nextChar == '-' && j+1 < len(sqlBatch) && sqlBatch[j+1] == '-' {
					// 找到了注释，添加到当前语句
					currentStatement.WriteByte(nextChar)      // 添加'-'
					currentStatement.WriteByte(sqlBatch[j+1]) // 添加第二个'-'
					j++

					// 读取直到行尾
					for k := j + 1; k < len(sqlBatch); k++ {
						commentChar := sqlBatch[k]
						currentStatement.WriteByte(commentChar)
						j = k

						if commentChar == '\n' || commentChar == '\r' {
							break
						}
					}
					i = j
					break
				} else if !isWhitespace(nextChar) {
					// 非注释且非空白字符，停止收集
					break
				} else {
					// 是空白字符，添加到当前语句
					currentStatement.WriteByte(nextChar)
					i = j
				}
			}

			statements = append(statements, strings.TrimSpace(currentStatement.String()))
			currentStatement.Reset()
			continue
		}

		escaped = false
	}

	// 处理最后一条可能没有分号的语句
	lastStatement := strings.TrimSpace(currentStatement.String())
	if lastStatement != "" {
		statements = append(statements, lastStatement)
	}

	return statements
}

// 判断字符是否为空白字符
func isWhitespace(c byte) bool {
	return unicode.IsSpace(rune(c))
}
