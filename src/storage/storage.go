package storage

import (
	"context"
	"fmt"
	"os"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
	"github.com/didi/nightingale/v5/src/pkg/ormx"
	"github.com/didi/nightingale/v5/src/pkg/tls"
)

type RedisConfig struct {
	Address  string
	Username string
	Password string
	DB       int
	UseTLS   bool
	tls.ClientConfig
}

var DB *gorm.DB

func InitDB(cfg ormx.DBConfig) error {
	db, err := ormx.New(cfg)
	if err == nil {
		DB = db
	}
	return err
}

var Redis *redis.Client

func InitRedis(cfg RedisConfig) (func(), error) {
	redisOptions := &redis.Options{
		Addr:     cfg.Address,
		Username: cfg.Username,
		Password: cfg.Password,
		DB:       cfg.DB,
	}

	if cfg.UseTLS {
		tlsConfig, err := cfg.TLSConfig()
		if err != nil {
			fmt.Println("failed to init redis tls config:", err)
			os.Exit(1)
		}
		redisOptions.TLSConfig = tlsConfig
	}

	Redis = redis.NewClient(redisOptions)

	err := Redis.Ping(context.Background()).Err()
	if err != nil {
		fmt.Println("failed to ping redis:", err)
		os.Exit(1)
	}

	return func() {
		fmt.Println("redis exiting")
		Redis.Close()
	}, nil
}
