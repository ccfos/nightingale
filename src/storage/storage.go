package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"

	"github.com/didi/nightingale/v5/src/pkg/ormx"
)

type RedisConfig struct {
	Address  string
	Password string
	DB       int
}

type DBConfig struct {
	Gorm     Gorm
	MySQL    MySQL
	Postgres Postgres
}

type Gorm struct {
	Debug             bool
	DBType            string
	MaxLifetime       int
	MaxOpenConns      int
	MaxIdleConns      int
	TablePrefix       string
	EnableAutoMigrate bool
}

type MySQL struct {
	Address    string
	User       string
	Password   string
	DBName     string
	Parameters string
}

func (a MySQL) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?%s",
		a.User, a.Password, a.Address, a.DBName, a.Parameters)
}

type Postgres struct {
	Address  string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func (a Postgres) DSN() string {
	arr := strings.Split(a.Address, ":")
	if len(arr) != 2 {
		panic("pg address(" + a.Address + ") invalid")
	}

	return fmt.Sprintf("host=%s port=%s user=%s dbname=%s password=%s sslmode=%s",
		arr[0], arr[1], a.User, a.DBName, a.Password, a.SSLMode)
}

var DB *gorm.DB

func InitDB(cfg DBConfig) error {
	db, err := newGormDB(cfg)
	if err == nil {
		DB = db
	}
	return err
}

func newGormDB(cfg DBConfig) (*gorm.DB, error) {
	var dsn string
	switch cfg.Gorm.DBType {
	case "mysql":
		dsn = cfg.MySQL.DSN()
	case "postgres":
		dsn = cfg.Postgres.DSN()
	default:
		return nil, errors.New("unknown DBType")
	}

	return ormx.New(ormx.Config{
		Debug:        cfg.Gorm.Debug,
		DBType:       cfg.Gorm.DBType,
		DSN:          dsn,
		MaxIdleConns: cfg.Gorm.MaxIdleConns,
		MaxLifetime:  cfg.Gorm.MaxLifetime,
		MaxOpenConns: cfg.Gorm.MaxOpenConns,
		TablePrefix:  cfg.Gorm.TablePrefix,
	})
}

var Redis *redis.Client

func InitRedis(cfg RedisConfig) (func(), error) {
	Redis = redis.NewClient(&redis.Options{
		Addr:     cfg.Address,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	err := Redis.Ping(context.Background()).Err()
	if err != nil {
		fmt.Println("ping redis failed:", err)
		os.Exit(1)
	}

	return func() {
		fmt.Println("redis exiting")
		Redis.Close()
	}, nil
}
