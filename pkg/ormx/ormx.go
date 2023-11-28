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

var t_log = logger.New(
	&TKitLogger{tklog.GetLogger()},
	logger.Config{
		SlowThreshold:             2 * time.Second,
		LogLevel:                  logger.Warn,
		IgnoreRecordNotFoundError: false,
		Colorful:                  true,
	},
)
var t_log_level map[string]logger.LogLevel

func init() {
	t_log_level = make(map[string]logger.LogLevel, 8)
	v := reflect.ValueOf(t_log).Elem()
	t_log_level[v.FieldByName("infoStr").String()] = logger.Info
	t_log_level[v.FieldByName("warnStr").String()] = logger.Warn
	t_log_level[v.FieldByName("errStr").String()] = logger.Error
	t_log_level[v.FieldByName("traceStr").String()] = logger.Info
	t_log_level[v.FieldByName("traceWarnStr").String()] = logger.Warn
	t_log_level[v.FieldByName("traceErrStr").String()] = logger.Error

}

type TKitLogger struct {
	logger *tklog.Logger
}

func (l *TKitLogger) Printf(s string, i ...interface{}) {
	level, ok := t_log_level[s]
	if !ok {
		l.logger.Debugf(s, i...)
	}
	switch level {
	case logger.Info:
		l.logger.Infof(s, i...)
	case logger.Warn:
		l.logger.Warningf(s, i...)
	case logger.Error:
		l.logger.Errorf(s, i...)
	default:
		l.logger.Debugf(s, i...)
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
		Logger: t_log,
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
