package redisc

import (
	"github.com/garyburd/redigo/redis"
	"github.com/toolkits/pkg/logger"
)

func HasKey(key string) bool {
	rc := RedisConnPool.Get()
	defer rc.Close()

	ret, _ := redis.Bool(rc.Do("EXISTS", key))

	return ret
}

func INCR(key string) int {
	rc := RedisConnPool.Get()
	defer rc.Close()

	ret, err := redis.Int(rc.Do("INCR", key))
	if err != nil {
		logger.Errorf("incr %s error: %v", key, err)
	}

	return ret
}

func GET(key string) int64 {
	rc := RedisConnPool.Get()
	defer rc.Close()

	ret, err := redis.Int64(rc.Do("GET", key))
	if err != nil {
		logger.Errorf("get %s error: %v", key, err)
	}

	return ret
}

func SetWithTTL(key string, value interface{}, ttl int) error {
	rc := RedisConnPool.Get()
	defer rc.Close()

	_, err := rc.Do("SET", key, value, "EX", ttl)
	return err
}

func Set(key string, value interface{}) error {
	rc := RedisConnPool.Get()
	defer rc.Close()

	_, err := rc.Do("SET", key, value)
	return err
}

func DelKey(key string) error {
	rc := RedisConnPool.Get()
	defer rc.Close()

	_, err := rc.Do("DEL", key)
	return err
}

func HSET(key string, field interface{}, value interface{}) (int64, error) {
	rc := RedisConnPool.Get()
	defer rc.Close()

	return redis.Int64(rc.Do("HSET", key, field, value))
}

func HKEYS(key string) ([]string, error) {
	rc := RedisConnPool.Get()
	defer rc.Close()

	return redis.Strings(rc.Do("HKEYS", key))
}

func HDEL(keys []interface{}) (int64, error) {
	rc := RedisConnPool.Get()
	defer rc.Close()

	return redis.Int64(rc.Do("HDEL", keys...))
}
