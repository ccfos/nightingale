package ormx

import (
	"fmt"
	"reflect"
	"strings"
	"time"

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

// New Create gorm.DB instance
func New(c DBConfig) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch strings.ToLower(c.DBType) {
	case "mysql":
		dialector = mysql.Open(c.DSN)
	case "postgres":
		dialector = postgres.Open(c.DSN)
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

	db, err := gorm.Open(dialector, gconfig)
	if err != nil {
		return nil, err
	}

	if c.Debug {
		db = db.Debug()
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxIdleConns(c.MaxIdleConns)
	sqlDB.SetMaxOpenConns(c.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Duration(c.MaxLifetime) * time.Second)

	return db, nil
}
