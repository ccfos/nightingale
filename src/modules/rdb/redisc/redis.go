package redisc

import (
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/modules/rdb/config"
)

var RedisConnPool *redis.Pool

func InitRedis() {
	cfg := config.Config

	if !cfg.Redis.Enable {
		return
	}

	addr := cfg.Redis.Addr
	pass := cfg.Redis.Pass
	maxIdle := cfg.Redis.Idle
	idleTimeout := 240 * time.Second

	connTimeout := time.Duration(cfg.Redis.Timeout.Conn) * time.Millisecond
	readTimeout := time.Duration(cfg.Redis.Timeout.Read) * time.Millisecond
	writeTimeout := time.Duration(cfg.Redis.Timeout.Write) * time.Millisecond

	RedisConnPool = &redis.Pool{
		MaxIdle:     maxIdle,
		IdleTimeout: idleTimeout,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", addr, redis.DialConnectTimeout(connTimeout), redis.DialReadTimeout(readTimeout), redis.DialWriteTimeout(writeTimeout))
			if err != nil {
				return nil, err
			}

			if pass != "" {
				if _, err := c.Do("AUTH", pass); err != nil {
					c.Close()
					logger.Error("redis auth fail")
					return nil, err
				}
			}

			return c, err
		},
		TestOnBorrow: PingRedis,
	}
}

func PingRedis(c redis.Conn, t time.Time) error {
	_, err := c.Do("ping")
	if err != nil {
		logger.Errorf("ping redis fail: %v", err)
	}
	return err
}

func CloseRedis() {
	if !config.Config.Redis.Enable {
		return
	}
	logger.Info("closing redis...")
	RedisConnPool.Close()
}
