package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/ccfos/nightingale/v6/pkg/tlsx"
	"github.com/redis/go-redis/v9"
	"github.com/toolkits/pkg/logger"
)

type RedisConfig struct {
	Address  string
	Username string
	Password string
	DB       int
	tlsx.ClientConfig
	RedisType         string
	MasterName        string
	SentinelUsername  string
	SentinelPassword  string
	DialTimeoutMills  int // default 5000 ms
	ReadTimeoutMills  int // default 3000 ms
	WriteTimeoutMills int // default 3000 ms
}

type Redis redis.Cmdable

func NewRedis(cfg RedisConfig) (Redis, error) {
	var redisClient Redis

	if cfg.DialTimeoutMills == 0 {
		cfg.DialTimeoutMills = 5000
	}

	if cfg.ReadTimeoutMills == 0 {
		cfg.ReadTimeoutMills = 3000
	}

	if cfg.WriteTimeoutMills == 0 {
		cfg.WriteTimeoutMills = 3000
	}

	switch cfg.RedisType {
	case "standalone", "":
		redisOptions := &redis.Options{
			Addr:         cfg.Address,
			Username:     cfg.Username,
			Password:     cfg.Password,
			DB:           cfg.DB,
			DialTimeout:  time.Duration(cfg.DialTimeoutMills) * time.Millisecond,
			ReadTimeout:  time.Duration(cfg.ReadTimeoutMills) * time.Millisecond,
			WriteTimeout: time.Duration(cfg.WriteTimeoutMills) * time.Millisecond,
		}

		if cfg.UseTLS {
			tlsConfig, err := cfg.TLSConfig()
			if err != nil {
				fmt.Println("failed to init redis tls config:", err)
				os.Exit(1)
			}
			redisOptions.TLSConfig = tlsConfig
		}

		redisClient = redis.NewClient(redisOptions)

	case "cluster":
		redisOptions := &redis.ClusterOptions{
			Addrs:        strings.Split(cfg.Address, ","),
			Username:     cfg.Username,
			Password:     cfg.Password,
			DialTimeout:  time.Duration(cfg.DialTimeoutMills) * time.Millisecond,
			ReadTimeout:  time.Duration(cfg.ReadTimeoutMills) * time.Millisecond,
			WriteTimeout: time.Duration(cfg.WriteTimeoutMills) * time.Millisecond,
		}

		if cfg.UseTLS {
			tlsConfig, err := cfg.TLSConfig()
			if err != nil {
				fmt.Println("failed to init redis tls config:", err)
				os.Exit(1)
			}
			redisOptions.TLSConfig = tlsConfig
		}

		redisClient = redis.NewClusterClient(redisOptions)

	case "sentinel":
		redisOptions := &redis.FailoverOptions{
			MasterName:       cfg.MasterName,
			SentinelAddrs:    strings.Split(cfg.Address, ","),
			Username:         cfg.Username,
			Password:         cfg.Password,
			DB:               cfg.DB,
			SentinelUsername: cfg.SentinelUsername,
			SentinelPassword: cfg.SentinelPassword,
			DialTimeout:      time.Duration(cfg.DialTimeoutMills) * time.Millisecond,
			ReadTimeout:      time.Duration(cfg.ReadTimeoutMills) * time.Millisecond,
			WriteTimeout:     time.Duration(cfg.WriteTimeoutMills) * time.Millisecond,
		}

		if cfg.UseTLS {
			tlsConfig, err := cfg.TLSConfig()
			if err != nil {
				fmt.Println("failed to init redis tls config:", err)
				os.Exit(1)
			}
			redisOptions.TLSConfig = tlsConfig
		}

		redisClient = redis.NewFailoverClient(redisOptions)

	case "miniredis":
		s, err := miniredis.Run()
		if err != nil {
			fmt.Println("failed to init miniredis:", err)
			os.Exit(1)
		}
		redisClient = redis.NewClient(&redis.Options{
			Addr: s.Addr(),
		})

	default:
		fmt.Println("failed to init redis , redis type is illegal:", cfg.RedisType)
		os.Exit(1)
	}

	err := redisClient.Ping(context.Background()).Err()
	if err != nil {
		fmt.Println("failed to ping redis:", err)
		os.Exit(1)
	}
	return redisClient, nil
}

func MGet(ctx context.Context, r Redis, keys []string) [][]byte {
	var vals [][]byte
	pipe := r.Pipeline()
	for _, key := range keys {
		pipe.Get(ctx, key)
	}
	cmds, _ := pipe.Exec(ctx)

	for i, key := range keys {
		cmd := cmds[i]
		if errors.Is(cmd.Err(), redis.Nil) {
			continue
		}

		if cmd.Err() != nil {
			logger.Errorf("failed to get key: %s, err: %s", key, cmd.Err())
			continue
		}
		val := []byte(cmd.(*redis.StringCmd).Val())
		vals = append(vals, val)
	}

	return vals
}

func MSet(ctx context.Context, r Redis, m map[string]interface{}, expiration time.Duration) error {
	pipe := r.Pipeline()
	for k, v := range m {
		pipe.Set(ctx, k, v, expiration)
	}
	_, err := pipe.Exec(ctx)
	return err
}
