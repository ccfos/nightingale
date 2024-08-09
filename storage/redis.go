package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/tlsx"

	"github.com/redis/go-redis/v9"
	"github.com/toolkits/pkg/logger"
)

type RedisConfig struct {
	Address  string
	Username string
	Password string
	DB       int
	UseTLS   bool
	tlsx.ClientConfig
	RedisType        string
	MasterName       string
	SentinelUsername string
	SentinelPassword string
}

type Redis redis.Cmdable

func NewRedis(cfg RedisConfig) (Redis, error) {
	var redisClient Redis
	switch cfg.RedisType {
	case "standalone", "":
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

		redisClient = redis.NewClient(redisOptions)

	case "cluster":
		redisOptions := &redis.ClusterOptions{
			Addrs:    strings.Split(cfg.Address, ","),
			Username: cfg.Username,
			Password: cfg.Password,
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

func MSet(ctx context.Context, r Redis, m map[string]interface{}) error {
	pipe := r.Pipeline()
	for k, v := range m {
		pipe.Set(ctx, k, v, 0)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// LPush push value to list
func LPush(ctx context.Context, r Redis, key string, values ...interface{}) error {

	_, err := r.LPush(ctx, key, values).Result()
	if err != nil {
		return err
	}
	return nil
}

func LTrim(ctx context.Context, r Redis, key string, start, stop int64) error {
	return r.LTrim(ctx, key, start, stop).Err()
}

func Expire(ctx context.Context, r Redis, key string, expiration time.Duration) error {
	return r.Expire(ctx, key, expiration).Err()
}

// MRangeList get multiple list from redis and unmarshal to []T
func MRangeList[T any](ctx context.Context, r Redis, keys []string) ([]T, error) {
	pipe := r.Pipeline()
	for _, k := range keys {
		pipe.LRange(ctx, k, 0, -1)
	}
	cmds, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}
	var res []T
	for i := range cmds {
		if cmds[i].Err() != nil {
			continue
		}
		val := cmds[i].(*redis.StringSliceCmd).Val()
		for _, v := range val {
			var temp T
			err := json.Unmarshal([]byte(v), &temp)
			if err != nil {
				continue
			}
			res = append(res, temp)
		}
	}
	return res, nil
}

func Scan(ctx context.Context, r Redis, cursor uint64, match string, count int64) ([]string, uint64, error) {
	return r.Scan(ctx, cursor, match, count).Result()
}

func MLLen(ctx context.Context, r Redis, keys []string) (map[string]int64, error) {
	pipe := r.Pipeline()
	for _, key := range keys {
		pipe.LLen(ctx, key)
	}
	cmds, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}
	res := make(map[string]int64)
	for i, key := range keys {
		cmd := cmds[i]
		if errors.Is(cmd.Err(), redis.Nil) {
			res[key] = 0
			continue
		}

		if cmd.Err() != nil {
			logger.Errorf("failed to get length of key: %s, err: %s", key, cmd.Err())
			continue
		}
		res[key] = cmd.(*redis.IntCmd).Val()
	}
	return res, nil
}

func LLen(ctx context.Context, r Redis, key string) (int64, error) {
	return r.LLen(ctx, key).Result()
}

func MTTL(ctx context.Context, r Redis, keys []string) (map[string]time.Duration, error) {
	pipe := r.Pipeline()
	for _, key := range keys {
		pipe.TTL(ctx, key)
	}
	cmds, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}
	res := make(map[string]time.Duration)
	for i, key := range keys {
		cmd := cmds[i]
		if errors.Is(cmd.Err(), redis.Nil) {
			continue
		}

		if cmd.Err() != nil {
			logger.Errorf("failed to get ttl of key: %s, err: %s", key, cmd.Err())
			continue
		}
		res[key] = cmd.(*redis.DurationCmd).Val()
	}
	return res, nil
}

func TTL(ctx context.Context, r Redis, key string) (time.Duration, error) {
	return r.TTL(ctx, key).Result()
}

func MDel(ctx context.Context, r Redis, keys ...string) error {
	pipe := r.Pipeline()
	for _, key := range keys {
		pipe.Del(ctx, key)
	}
	_, err := pipe.Exec(ctx)
	return err
}
