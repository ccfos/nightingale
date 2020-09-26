package redisc

import (
	"log"
	"time"

	"github.com/didi/nightingale/src/modules/monapi/config"

	"github.com/garyburd/redigo/redis"
	"github.com/toolkits/pkg/logger"
)

var RedisConnPool *redis.Pool

func InitRedis() {
	cfg := config.Get()

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
				logger.Errorf("conn redis err:%v", err)
				return nil, err
			}

			if pass != "" {
				if _, err := c.Do("AUTH", pass); err != nil {
					c.Close()
					logger.Errorf("ERR: redis auth fail:%v", err)
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
		log.Println("ERR: ping redis fail", err)
	}
	return err
}

func CloseRedis() {
	log.Println("INFO: closing redis...")
	RedisConnPool.Close()
}
