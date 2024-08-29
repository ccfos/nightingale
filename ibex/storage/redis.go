package storage

import (
	"context"
	"time"

	"github.com/ccfos/nightingale/v6/storage"
	"github.com/redis/go-redis/v9"
)

type Redis redis.Cmdable

var Cache Redis

const DEFAULT = time.Hour

func InitRedis(cfg storage.RedisConfig) (err error) {
	Cache, err = storage.NewRedis(cfg)
	if err != nil {
		return err
	}

	return IdInit()
}

func CacheMGet(ctx context.Context, keys []string) [][]byte {
	return storage.MGet(ctx, Cache, keys)
}

const IDINITIAL = 1 << 32

func IdInit() error {
	return Cache.Set(context.Background(), "id", IDINITIAL, 0).Err()
}

func IdGet() (int64, error) {
	return Cache.Incr(context.Background(), "id").Result()
}
