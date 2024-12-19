package ormx

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	tklog "github.com/toolkits/pkg/logger"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// DBConfig GORM DBConfig
type DBConfig struct {
	Debug        bool
	DBType       string
	DSN          string
	MaxLifetime  int
	MaxOpenConns int
	MaxIdleConns int
	TablePrefix  string
}

var gormLogger = logger.New(
	&TKitLogger{tklog.GetLogger()},
	logger.Config{
		SlowThreshold:             2 * time.Second,
		LogLevel:                  logger.Warn,
		IgnoreRecordNotFoundError: false,
		Colorful:                  true,
	},
)
var logLevelMap map[string]logger.LogLevel

func init() {
	logLevelMap = make(map[string]logger.LogLevel, 8)
	v := reflect.ValueOf(gormLogger).Elem()
	logLevelMap[v.FieldByName("infoStr").String()] = logger.Info
	logLevelMap[v.FieldByName("warnStr").String()] = logger.Warn
	logLevelMap[v.FieldByName("errStr").String()] = logger.Error
	logLevelMap[v.FieldByName("traceStr").String()] = logger.Info
	logLevelMap[v.FieldByName("traceWarnStr").String()] = logger.Warn
	logLevelMap[v.FieldByName("traceErrStr").String()] = logger.Error

}

type TKitLogger struct {
	writer *tklog.Logger
}

func (l *TKitLogger) Printf(s string, i ...interface{}) {
	level, ok := logLevelMap[s]
	if !ok {
		l.writer.Debugf(s, i...)
	}
	switch level {
	case logger.Info:
		l.writer.Infof(s, i...)
	case logger.Warn:
		l.writer.Warningf(s, i...)
	case logger.Error:
		l.writer.Errorf(s, i...)
	default:
		l.writer.Debugf(s, i...)
	}
}

func createDatabase(c DBConfig, gconfig *gorm.Config) error {
	switch strings.ToLower(c.DBType) {
	case "mysql":
		return createMysqlDatabase(c.DSN, gconfig)
	case "postgres":
		return createPostgresDatabase(c.DSN, gconfig)
	case "sqlite":
		return createSqliteDatabase(c.DSN, gconfig)
	default:
		return fmt.Errorf("dialector(%s) not supported", c.DBType)
	}
}

func createSqliteDatabase(dsn string, gconfig *gorm.Config) error {
	tempDialector := sqlite.Open(dsn)

	_, err := gorm.Open(tempDialector, gconfig)
	if err != nil {
		return fmt.Errorf("failed to open temporary connection: %v", err)
	}

	fmt.Println("sqlite file created")

	return nil
}

func createPostgresDatabase(dsn string, gconfig *gorm.Config) error {
	dsnParts := strings.Split(dsn, " ")
	dbName := ""
	connectionWithoutDB := ""
	for _, part := range dsnParts {
		if strings.HasPrefix(part, "dbname=") {
			dbName = part[strings.Index(part, "=")+1:]
		} else {
			connectionWithoutDB += part
			connectionWithoutDB += " "
		}
	}

	createDBQuery := fmt.Sprintf("CREATE DATABASE %s ENCODING='UTF8' LC_COLLATE='en_US.utf8' LC_CTYPE='en_US.utf8';", dbName)

	tempDialector := postgres.Open(connectionWithoutDB)

	tempDB, err := gorm.Open(tempDialector, gconfig)
	if err != nil {
		return fmt.Errorf("failed to open temporary connection: %v", err)
	}

	result := tempDB.Exec(createDBQuery)
	if result.Error != nil {
		return fmt.Errorf("failed to execute create database query: %v", result.Error)
	}

	return nil
}

func createMysqlDatabase(dsn string, gconfig *gorm.Config) error {
	dsnParts := strings.SplitN(dsn, "/", 2)
	if len(dsnParts) != 2 {
		return fmt.Errorf("failed to parse DSN: %s", dsn)
	}

	connectionInfo := dsnParts[0]
	dbInfo := dsnParts[1]
	dbName := dbInfo

	queryIndex := strings.Index(dbInfo, "?")
	if queryIndex != -1 {
		dbName = dbInfo[:queryIndex]
	} else {
		return fmt.Errorf("failed to parse database name from DSN: %s", dsn)
	}

	connectionWithoutDB := connectionInfo + "/?" + dbInfo[queryIndex+1:]
	createDBQuery := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s CHARACTER SET utf8mb4", dbName)

	tempDialector := mysql.Open(connectionWithoutDB)

	tempDB, err := gorm.Open(tempDialector, gconfig)
	if err != nil {
		return fmt.Errorf("failed to open temporary connection: %v", err)
	}

	result := tempDB.Exec(createDBQuery)
	if result.Error != nil {
		return fmt.Errorf("failed to execute create database query: %v", result.Error)
	}

	return nil
}

func checkDatabaseExist(c DBConfig) (bool, error) {
	switch strings.ToLower(c.DBType) {
	case "mysql":
		return checkMysqlDatabaseExist(c)
	case "postgres":
		return checkPostgresDatabaseExist(c)
	case "sqlite":
		return checkSqliteDatabaseExist(c)
	default:
		return false, fmt.Errorf("dialector(%s) not supported", c.DBType)
	}

}

func checkSqliteDatabaseExist(c DBConfig) (bool, error) {
	if _, err := os.Stat(c.DSN); os.IsNotExist(err) {
		fmt.Printf("sqlite file not exists: %s\n", c.DSN)
		return false, nil
	} else {
		return true, nil
	}
}

func checkPostgresDatabaseExist(c DBConfig) (bool, error) {
	dsnParts := strings.Split(c.DSN, " ")
    dbName := ""
    dbpair := ""
    for _, part := range dsnParts {
        if strings.HasPrefix(part, "dbname=") {
            dbName = part[strings.Index(part, "=")+1:]
            dbpair = part
        }
    }
    connectionStr := strings.Replace(c.DSN, dbpair, "dbname=postgres", 1)
    dialector := postgres.Open(connectionStr)

	gconfig := &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   c.TablePrefix,
			SingularTable: true,
		},
		Logger: gormLogger,
	}

	db, err := gorm.Open(dialector, gconfig)
	if err != nil {
		return false, fmt.Errorf("failed to open database: %v", err)
	}

	var databases []string
	query := genQuery(c)
	if err := db.Raw(query).Scan(&databases).Error; err != nil {
		return false, fmt.Errorf("failed to query: %v", err)
	}

	for _, database := range databases {
		if database == dbName {
			fmt.Println("Database exist")
			return true, nil
		}
	}

	return false, nil
}

func checkMysqlDatabaseExist(c DBConfig) (bool, error) {
	dsnParts := strings.SplitN(c.DSN, "/", 2)
	if len(dsnParts) != 2 {
		return false, fmt.Errorf("failed to parse DSN: %s", c.DSN)
	}

	connectionInfo := dsnParts[0]
	dbInfo := dsnParts[1]
	dbName := dbInfo

	queryIndex := strings.Index(dbInfo, "?")
	if queryIndex != -1 {
		dbName = dbInfo[:queryIndex]
	} else {
		return false, fmt.Errorf("failed to parse database name from DSN: %s", c.DSN)
	}

	connectionWithoutDB := connectionInfo + "/?" + dbInfo[queryIndex+1:]

	var dialector gorm.Dialector
	switch strings.ToLower(c.DBType) {
	case "mysql":
		dialector = mysql.Open(connectionWithoutDB)
	case "postgres":
		dialector = postgres.Open(connectionWithoutDB)
	default:
		return false, fmt.Errorf("unsupported database type: %s", c.DBType)
	}

	gconfig := &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   c.TablePrefix,
			SingularTable: true,
		},
		Logger: gormLogger,
	}

	db, err := gorm.Open(dialector, gconfig)
	if err != nil {
		return false, fmt.Errorf("failed to open database: %v", err)
	}

	var databases []string
	query := genQuery(c)
	if err := db.Raw(query).Scan(&databases).Error; err != nil {
		return false, fmt.Errorf("failed to query: %v", err)
	}

	for _, database := range databases {
		if database == dbName {
			return true, nil
		}
	}

	return false, nil
}

func genQuery(c DBConfig) string {
	switch strings.ToLower(c.DBType) {
	case "mysql":
		return "SHOW DATABASES"
	case "postgres":
		return "SELECT datname FROM pg_database"
	case "sqlite":
		return ""
	default:
		return ""
	}
}

// New Create gorm.DB instance
func New(c DBConfig) (*gorm.DB, error) {
	var dialector gorm.Dialector
	sqliteUsed := false

	switch strings.ToLower(c.DBType) {
	case "mysql":
		dialector = mysql.Open(c.DSN)
	case "postgres":
		dialector = postgres.Open(c.DSN)
	case "sqlite":
		dialector = sqlite.Open(c.DSN)
		sqliteUsed = true
	default:
		return nil, fmt.Errorf("dialector(%s) not supported", c.DBType)
	}

	gconfig := &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   c.TablePrefix,
			SingularTable: true,
		},
		Logger: gormLogger,
	}

	dbExist, checkErr := checkDatabaseExist(c)
	if checkErr != nil {
		return nil, checkErr
	}
	if !dbExist {
		fmt.Println("Database not exist, trying to create it")
		createErr := createDatabase(c, gconfig)
		if createErr != nil {
			return nil, fmt.Errorf("failed to create database: %v", createErr)
		}

		db, err := gorm.Open(dialector, gconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to reopen database after creation: %v", err)
		}
		err = DataBaseInit(c, db)
		if err != nil {
			return nil, fmt.Errorf("failed to init database: %v", err)
		}
	}

	db, err := gorm.Open(dialector, gconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	if c.Debug {
		db = db.Debug()
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	if !sqliteUsed {
		sqlDB.SetMaxIdleConns(c.MaxIdleConns)
		sqlDB.SetMaxOpenConns(c.MaxOpenConns)
		sqlDB.SetConnMaxLifetime(time.Duration(c.MaxLifetime) * time.Second)
	}

	return db, nil
}
