package redisc

import (
	"time"

	"github.com/garyburd/redigo/redis"

	"github.com/toolkits/pkg/logger"
)

var RedisConnPool *redis.Pool

type RedisSection struct {
	Local localRedis `yaml:"local"`
}

type localRedis struct {
	Enable  bool           `yaml:"enable"`
	Addr    string         `yaml:"addr"`
	Pass    string         `yaml:"pass"`
	Idle    int            `yaml:"idle"`
	Timeout timeoutSection `yaml:"timeout"`
}

type timeoutSection struct {
	Conn  int `yaml:"conn"`
	Read  int `yaml:"read"`
	Write int `yaml:"write"`
}

func InitRedis(r RedisSection) {
	cfg := r.Local

	addr := cfg.Addr
	pass := cfg.Pass
	maxIdle := cfg.Idle
	idleTimeout := 240 * time.Second

	connTimeout := time.Duration(cfg.Timeout.Conn) * time.Millisecond
	readTimeout := time.Duration(cfg.Timeout.Read) * time.Millisecond
	writeTimeout := time.Duration(cfg.Timeout.Write) * time.Millisecond

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
	logger.Info("closing redis...")
	RedisConnPool.Close()
}
