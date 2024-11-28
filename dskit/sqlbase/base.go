// @Author: Ciusyan 5/19/24

package sqlbase

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/dskit/types"

	"gorm.io/gorm"
)

// NewDB creates a new Gorm DB instance based on the provided gorm.Dialector and configures the connection pool
func NewDB(ctx context.Context, dialector gorm.Dialector, maxIdleConns, maxOpenConns int, connMaxLifetime time.Duration) (*gorm.DB, error) {
	// Create a new Gorm DB instance
	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// Configure the connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetConnMaxLifetime(connMaxLifetime)

	return db.WithContext(ctx), sqlDB.Ping()
}

// ShowTables retrieves a list of all tables in the specified database
func ShowTables(ctx context.Context, db *gorm.DB, query string) ([]string, error) {
	var tables []string

	rows, err := db.WithContext(ctx).Raw(query).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}

	return tables, nil
}

// ShowDatabases retrieves a list of all databases in the connected database server
func ShowDatabases(ctx context.Context, db *gorm.DB, query string) ([]string, error) {
	var databases []string

	rows, err := db.WithContext(ctx).Raw(query).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var database string
		if err := rows.Scan(&database); err != nil {
			return nil, err
		}
		databases = append(databases, database)
	}

	return databases, nil
}

// DescTable describes the schema of a specified table in MySQL or PostgreSQL
func DescTable(ctx context.Context, db *gorm.DB, query string) ([]*types.ColumnProperty, error) {
	rows, err := db.WithContext(ctx).Raw(query).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []*types.ColumnProperty
	for rows.Next() {
		var (
			field        string
			typ          string
			null         string
			key          sql.NullString
			defaultValue sql.NullString
			extra        sql.NullString
		)

		switch db.Dialector.Name() {
		case "mysql":
			if err := rows.Scan(&field, &typ, &null, &key, &defaultValue, &extra); err != nil {
				continue
			}
		case "postgres", "sqlserver":
			if err := rows.Scan(&field, &typ, &null, &defaultValue); err != nil {
				continue
			}
		case "oracle":
			if err := rows.Scan(&field, &typ, &null); err != nil {
				continue
			}
		}

		// Convert the database-specific type to internal type
		type2, indexable := convertDBType(db.Dialector.Name(), typ)
		columns = append(columns, &types.ColumnProperty{
			Field:     field,
			Type:      typ,
			Type2:     type2,
			Indexable: indexable,
		})
	}
	return columns, nil
}

// ExecQuery executes the specified query and returns the result rows
func ExecQuery(ctx context.Context, db *gorm.DB, sql string) ([]map[string]interface{}, error) {
	rows, err := db.WithContext(ctx).Raw(sql).Rows()
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

// SelectRows selects rows from a specified table based on a given query
func SelectRows(ctx context.Context, db *gorm.DB, table, query string) ([]map[string]interface{}, error) {
	sql := fmt.Sprintf("SELECT * FROM %s", table)
	if query != "" {
		sql += " WHERE " + query
	}

	return ExecQuery(ctx, db, sql)
}

// convertDBType converts MySQL or PostgreSQL data types to custom internal types and determines if they are indexable
func convertDBType(dialect, dbType string) (string, bool) {
	typ := strings.ToLower(dbType)

	// Common type conversions
	switch {
	case strings.HasPrefix(typ, "int"), strings.HasPrefix(typ, "tinyint"),
		strings.HasPrefix(typ, "smallint"), strings.HasPrefix(typ, "mediumint"),
		strings.HasPrefix(typ, "bigint"), strings.HasPrefix(typ, "serial"),
		strings.HasPrefix(typ, "bigserial"):
		return types.LogExtractValueTypeLong, true

	case strings.HasPrefix(typ, "varchar"), strings.HasPrefix(typ, "text"),
		strings.HasPrefix(typ, "char"), strings.HasPrefix(typ, "tinytext"),
		strings.HasPrefix(typ, "mediumtext"), strings.HasPrefix(typ, "longtext"),
		strings.HasPrefix(typ, "character varying"), strings.HasPrefix(typ, "nvarchar"),
		strings.HasPrefix(typ, "nchar"):
		return types.LogExtractValueTypeText, true

	case strings.HasPrefix(typ, "float"), strings.HasPrefix(typ, "double"),
		strings.HasPrefix(typ, "decimal"), strings.HasPrefix(typ, "numeric"),
		strings.HasPrefix(typ, "real"), strings.HasPrefix(typ, "double precision"):
		return types.LogExtractValueTypeFloat, true

	case strings.HasPrefix(typ, "date"), strings.HasPrefix(typ, "datetime"),
		strings.HasPrefix(typ, "timestamp"), strings.HasPrefix(typ, "timestamptz"),
		strings.HasPrefix(typ, "time"), strings.HasPrefix(typ, "smalldatetime"):
		return types.LogExtractValueTypeDate, false

	case strings.HasPrefix(typ, "boolean"), strings.HasPrefix(typ, "bit"):
		return types.LogExtractValueTypeBool, false
	}

	// Specific type conversions for MySQL
	if dialect == "mysql" {
		switch {
		default:
			return typ, false
		}
	}

	// Specific type conversions for PostgreSQL
	if dialect == "postgres" {
		switch {
		default:
			return typ, false
		}
	}

	if dialect == "oracle" {
		switch {
		default:
			return typ, false
		}
	}

	// Can continue to add specific 'dialect' type ...

	return typ, false
}
