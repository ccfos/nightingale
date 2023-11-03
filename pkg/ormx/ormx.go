package ormx

import (
	"fmt"
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
type TKitLogger struct {
	logger *tklog.Logger
}

func (l *TKitLogger) Printf(s string, i ...interface{}) {
	l.logger.Warningf(s, i...)
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
		Logger: logger.New(
			&TKitLogger{tklog.GetLogger()},
			logger.Config{
				SlowThreshold:             2 * time.Second,
				LogLevel:                  logger.Warn,
				IgnoreRecordNotFoundError: false,
				Colorful:                  true,
			},
		),
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
