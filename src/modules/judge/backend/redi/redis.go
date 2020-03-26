package redi

import (
	"log"
	"time"

	"github.com/didi/nightingale/src/toolkits/stats"
	"github.com/garyburd/redigo/redis"
	"github.com/toolkits/pkg/logger"
)

var RedisConnPools []*redis.Pool
var Config RedisSection

type RedisSection struct {
	Addrs   []string       `yaml:"addrs"`
	Pass    string         `yaml:"pass"`
	DB      int            `yaml:"db"`
	Idle    int            `yaml:"idle"`
	Timeout TimeoutSection `yaml:"timeout"`
	Prefix  string         `yaml:"prefix"`
}

type TimeoutSection struct {
	Conn  int `yaml:"conn"`
	Read  int `yaml:"read"`
	Write int `yaml:"write"`
}

func Init(cfg RedisSection) {
	Config = cfg

	addrs := cfg.Addrs
	pass := cfg.Pass
	db := cfg.DB
	maxIdle := cfg.Idle
	idleTimeout := 240 * time.Second

	connTimeout := time.Duration(cfg.Timeout.Conn) * time.Millisecond
	readTimeout := time.Duration(cfg.Timeout.Read) * time.Millisecond
	writeTimeout := time.Duration(cfg.Timeout.Write) * time.Millisecond
	for _, addr := range addrs {
		redisConnPool := &redis.Pool{
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
						logger.Errorf("ERR: redis auth fail:%v", err)
						stats.Counter.Set("redis.conn.failed", 1)

						return nil, err
					}
				}

				if db != 0 {
					if _, err := c.Do("SELECT", db); err != nil {
						c.Close()
						logger.Error("redis select db fail, db: ", db)
						stats.Counter.Set("redis.conn.failed", 1)
						return nil, err
					}
				}

				return c, err
			},
			TestOnBorrow: PingRedis,
		}
		RedisConnPools = append(RedisConnPools, redisConnPool)
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
	for i := range RedisConnPools {
		RedisConnPools[i].Close()
	}
}
