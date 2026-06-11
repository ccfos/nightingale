// @Author: Ciusyan 5/19/24

package sqlbase

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/ccfos/nightingale/v6/dskit/types"
)

// NewDB creates a new Gorm DB instance based on the provided gorm.Dialector and configures the connection pool
func NewDB(ctx context.Context, dialector gorm.Dialector, maxIdleConns, maxOpenConns int, connMaxLifetime time.Duration) (*gorm.DB, error) {
	// Create a new Gorm DB instance
	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return db, err
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

func CloseDB(db *gorm.DB) error {
	if db != nil {
		sqlDb, err := db.DB()
		if err != nil {
			return err
		}
		return sqlDb.Close()
	}
	return nil
}

// ShowTables retrieves a list of all tables in the specified database
func ShowTables(ctx context.Context, db *gorm.DB, query string, args ...interface{}) ([]string, error) {
	tables := make([]string, 0)

	rows, err := db.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return nil, err
	}
	// FIX: 增加nil指针防御
	if rows == nil {
		return nil, errors.New("empty rows returned")
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
func ShowDatabases(ctx context.Context, db *gorm.DB, query string, args ...interface{}) ([]string, error) {
	var databases []string

	rows, err := db.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return nil, err
	}
	// FIX: 增加nil指针防御
	if rows == nil {
		return nil, errors.New("empty rows returned")
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
func DescTable(ctx context.Context, db *gorm.DB, query string, args ...interface{}) ([]*types.ColumnProperty, error) {
	rows, err := db.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return nil, err
	}
	// FIX: 增加nil指针防御
	if rows == nil {
		return nil, errors.New("empty rows returned")
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
		type2, indexable := ConvertDBType(db.Dialector.Name(), typ)
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
func ExecQuery(ctx context.Context, db *gorm.DB, sql string, args ...interface{}) ([]map[string]interface{}, error) {
	rows, err := db.WithContext(ctx).Raw(sql, args...).Rows()
	if err != nil {
		return nil, err
	}
	// FIX: 增加nil指针防御
	if rows == nil {
		return nil, errors.New("empty rows returned")
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

// convertDBType converts MySQL or PostgreSQL data types to custom internal types and determines if they are indexable
func ConvertDBType(dialect, dbType string) (string, bool) {
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
		strings.HasPrefix(typ, "nchar"), strings.HasPrefix(typ, "bpchar"):
		return types.LogExtractValueTypeText, true

	case strings.HasPrefix(typ, "float"), strings.HasPrefix(typ, "double"),
		strings.HasPrefix(typ, "decimal"), strings.HasPrefix(typ, "numeric"),
		strings.HasPrefix(typ, "real"), strings.HasPrefix(typ, "double precision"):
		return types.LogExtractValueTypeFloat, true

	case strings.HasPrefix(typ, "date"), strings.HasPrefix(typ, "datetime"),
		strings.HasPrefix(typ, "timestamp"), strings.HasPrefix(typ, "timestamptz"),
		strings.HasPrefix(typ, "time"), strings.HasPrefix(typ, "smalldatetime"):
		return types.LogExtractValueTypeDate, false

	case strings.HasPrefix(typ, "boolean"), strings.HasPrefix(typ, "bit"), strings.HasPrefix(typ, "bool"):
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
