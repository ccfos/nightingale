package redisc

import (
	"encoding/json"
	"fmt"
	"github.com/didi/nightingale/v4/src/models"
	"strings"

	"github.com/didi/nightingale/v4/src/common/dataobj"
	"github.com/didi/nightingale/v4/src/common/stats"

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

func Push(event *dataobj.Event) error {
	bytes, err := json.Marshal(event)
	if err != nil {
		err = fmt.Errorf("redis publish failed, error:%v", err)
		return err
	}

	rc := RedisConnPool.Get()
	defer rc.Close()

	// 如果写入用lpush 则读出应该用 rpop
	// 如果写入用rpush 则读出应该用 lpop
	stats.Counter.Set("redis.push", 1)
	_, err = rc.Do("LPUSH", event.Partition, string(bytes))
	if err == nil {
		logger.Debugf("redis publish succ, event: %s", string(bytes))
		return nil
	}

	return fmt.Errorf("redis publish failed finally:%v", err)
}

func Pop(count int, queue string) []*dataobj.Message {
	var ret []*dataobj.Message

	rc := RedisConnPool.Get()
	defer rc.Close()

	for i := 0; i < count; i++ {
		reply, err := redis.String(rc.Do("RPOP", queue))
		if err != nil {
			if err != redis.ErrNil {
				logger.Errorf("rpop queue:%s failed, err: %v", queue, err)
			}
			break
		}

		if reply == "" || reply == "nil" {
			continue
		}

		var message dataobj.Message
		err = json.Unmarshal([]byte(reply), &message)
		if err != nil {
			logger.Errorf("unmarshal message failed, err: %v, redis reply: %v", err, reply)
			continue
		}

		ret = append(ret, &message)
	}

	return ret
}

func PopEvent(count int, queues []interface{}) []*models.Event {
	queues = append(queues, 1)

	var ret []*models.Event

	rc := RedisConnPool.Get()
	defer rc.Close()

	for i := 0; i < count; i++ {
		reply, err := redis.Strings(rc.Do("BRPOP", queues...))
		if err != nil {
			if err != redis.ErrNil {
				logger.Errorf("brpop queue:%s failed, err: %v", queues, err)
			}
			break
		}

		if reply == nil {
			continue
		}

		var event models.Event
		err = json.Unmarshal([]byte(reply[1]), &event)
		if err != nil {
			logger.Errorf("unmarshal event failed, err: %v, redis reply: %v", err, reply)
			continue
		}

		ret = append(ret, &event)
	}

	return ret
}

func lpush(queue, message string) error {
	rc := RedisConnPool.Get()
	defer rc.Close()
	_, err := rc.Do("LPUSH", queue, message)
	if err != nil {
		logger.Errorf("LPUSH %s fail, message:%s, error:%v", queue, message, err)
	}
	return err
}

// Write LPUSH message to redis
func Write(data *dataobj.Message, queue string) error {
	if data == nil {
		return fmt.Errorf("message is nil")
	}

	data.Tos = removeEmptyString(data.Tos)

	bs, err := json.Marshal(data)
	if err != nil {
		logger.Errorf("marshal message failed, message: %+v, err: %v", data, err)
		return err
	}

	logger.Debugf("write message to queue, message:%+v, queue:%s", data, queue)
	return lpush(queue, string(bs))
}

func removeEmptyString(s []string) []string {
	cnt := len(s)
	ss := make([]string, 0, cnt)
	for i := 0; i < cnt; i++ {
		if strings.TrimSpace(s[i]) == "" {
			continue
		}

		ss = append(ss, s[i])
	}

	return ss
}
