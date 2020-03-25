package redisc

import (
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/modules/monapi/config"
	"github.com/didi/nightingale/src/toolkits/stats"
)

var RedisConnPool *redis.Pool

func InitRedis() {
	cfg := config.Get()

	addr := cfg.Redis.Addr
	pass := cfg.Redis.Pass
	db := cfg.Redis.DB
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
				logger.Errorf("conn redis err:%v", err)
				stats.Counter.Set("redis.conn.failed", 1)
				return nil, err
			}

			if pass != "" {
				if _, err := c.Do("AUTH", pass); err != nil {
					c.Close()
					logger.Error("redis auth fail, pass: ", pass)
					stats.Counter.Set("redis.conn.failed", 1)
					return nil, err
				}
			}

			if db != 0 {
				if _, err := c.Do("SELECT", db); err != nil {
					c.Close()
					logger.Error("redis select db fail, db: ", db)
					stats.Counter.Set("redis.select.failed", 1)
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
		logger.Error("ping redis fail: ", err)
	}
	return err
}

func CloseRedis() {
	logger.Info("closing redis...")
	RedisConnPool.Close()
}
